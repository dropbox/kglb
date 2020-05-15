package health_manager

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dropbox/dlog"
	"dropbox/exclog"
	"dropbox/kglb/common"
	"dropbox/kglb/utils/concurrency"
	"dropbox/kglb/utils/discovery"
	"dropbox/kglb/utils/dns_resolver"
	"dropbox/kglb/utils/health_checker"
	pb "dropbox/proto/kglb"
	hc_pb "dropbox/proto/kglb/healthchecker"
	"dropbox/vortex2/v2stats"
	"godropbox/errors"
)

const (
	defaultHealtchCheckInterval = time.Second * 5
	defaultConcurrencyLimit     = uint32(100)
	defaultRiseCount            = 3
	defaultFallCount            = 2
)

type HealthManagerParams struct {
	UpstreamCheckerAttributes *hc_pb.UpstreamChecker

	// HealthManager ID. Used mostly for human readable log messages.
	Id string

	// Tags for statistic. All optional.
	// If not set - "unknown" will be used.
	SetupName   string
	ServiceName string

	// Resolver instance to discovery entries.
	Resolver discovery.DiscoveryResolver

	// Dns Resolver to resolve discovered hosts to ip
	DnsResolver dns_resolver.DnsResolver

	// HealthChecker implementation to perform actual health validation.
	HealthChecker health_checker.HealthChecker

	// Address Family of the vip/balancer to whom this health check manager belongs
	AddressFamily pb.AddressFamily

	// Initial health status of the just discovered HostPort entries.
	InitialHealthyState bool
}

type HealthManager struct {
	params *HealthManagerParams

	// chan to notify subscribers about state change.
	updateChan chan HealthManagerState
	// chan to notify healthCheckLoop about configuration changes.
	updateConfChan chan struct{}

	ctx        context.Context
	cancelFunc context.CancelFunc

	countLock sync.RWMutex
	statLock  sync.Mutex
	riseCount int
	fallCount int

	interval         atomic.Value // time.Duration
	concurrencyLimit uint32       // atomic
	resolver         atomic.Value // DiscoveryResolver
	checker          atomic.Value // HealthChecker

	// v2 counter maps: by hostname
	passCounters statCounterMap
	failCounters statCounterMap

	// v2 gauges
	aliveGauge v2stats.Gauge

	// Health manager state.
	state HealthManagerState
	// last state sent via updateChan.
	lastUpdateState atomic.Value // HealthManagerState

	// boolean flag for bypassing state change check before sending it to chan.
	// (true indicates that the state was sent).
	initialStateSent bool

	// boolean flag to help properly handle initial state from resolver.
	initialResolverStateRecv bool
}

type statCounterMap map[string]*v2stats.Counter

func NewHealthManager(ctx context.Context, params HealthManagerParams) (*HealthManager, error) {
	if params.Resolver == nil {
		return nil, errors.Newf("Resolver param cannot be empty: %+v", params)
	}
	if params.HealthChecker == nil {
		return nil, errors.Newf("HealthChecker param cannot be empty: %+v", params)
	}
	if params.UpstreamCheckerAttributes == nil {
		return nil, errors.Newf("UpstreamCheckerAttributes param cannot be empty: %+v", params)
	}

	// Apply default values for v2stat tags: setup / service names
	if params.ServiceName == "" {
		params.ServiceName = "unknown"
	}
	if params.SetupName == "" {
		params.SetupName = "unknown"
	}

	mng := &HealthManager{
		params:         &params,
		updateChan:     make(chan HealthManagerState, 1),
		updateConfChan: make(chan struct{}, 1),
		passCounters:   make(statCounterMap),
		failCounters:   make(statCounterMap),
		aliveGauge: aliveRatioGauge.Must(v2stats.KV{
			"setup":   params.SetupName,
			"service": params.ServiceName,
		}),
	}
	mng.ctx, mng.cancelFunc = context.WithCancel(ctx)

	mng.resolver.Store(params.Resolver)
	mng.checker.Store(params.HealthChecker)
	mng.lastUpdateState.Store(HealthManagerState{})

	if err := mng.Update(params.UpstreamCheckerAttributes); err != nil {
		return nil, err
	}
	// starting health checking loop.
	go mng.healthCheckLoop()

	return mng, nil
}

// Health manager id.
func (h *HealthManager) GetId() string {
	return h.params.Id
}

// Returns recent Health Manager state.
func (h *HealthManager) GetState() HealthManagerState {
	return h.lastUpdateState.Load().(HealthManagerState)
}

// Stop Health Manager.
func (h *HealthManager) Close() {
	h.cancelFunc()
}

func (h *HealthManager) Update(upstreamChecker *hc_pb.UpstreamChecker) error {
	riseCount := upstreamChecker.GetRiseCount()
	fallCount := upstreamChecker.GetFallCount()

	// Use defaults if both riseCount and fallCount are not provided in configuration
	if riseCount == 0 && fallCount == 0 {
		riseCount = defaultRiseCount
		fallCount = defaultFallCount
	}

	if riseCount <= 0 {
		return fmt.Errorf("RiseCount must be a positive number: %d", riseCount)
	}
	if fallCount <= 0 {
		return fmt.Errorf("FallCount must be a positive number: %d", fallCount)
	}

	interval := defaultHealtchCheckInterval
	if intervalMs := upstreamChecker.GetIntervalMs(); intervalMs > 0 {
		interval = time.Duration(intervalMs) * time.Millisecond
	}

	concurrencyLimit := defaultConcurrencyLimit
	if configuredConcurrencyLimit := upstreamChecker.GetConcurrencyLimit(); configuredConcurrencyLimit > 0 {
		concurrencyLimit = configuredConcurrencyLimit
	}

	h.countLock.Lock()
	defer h.countLock.Unlock()
	h.riseCount = int(riseCount)
	h.fallCount = int(fallCount)
	h.interval.Store(interval)
	atomic.StoreUint32(&h.concurrencyLimit, concurrencyLimit)

	select {
	case h.updateConfChan <- struct{}{}:
	default:
	}
	return nil
}

// Updates HealthChecker.
func (h *HealthManager) UpdateHealthChecker(checker health_checker.HealthChecker) {
	h.checker.Store(checker)
	// notify healthCheckLoop about the change.
	select {
	case h.updateConfChan <- struct{}{}:
	default:
	}
}

// Updates DiscoveryResolver.
func (h *HealthManager) UpdateResolver(resolver discovery.DiscoveryResolver) {
	h.resolver.Store(resolver)
	// notify healthCheckLoop about the change.
	select {
	case h.updateConfChan <- struct{}{}:
	default:
	}
}

func (h *HealthManager) GetCheckerConfiguration() *hc_pb.HealthCheckerAttributes {
	checker := h.checker.Load().(health_checker.HealthChecker)
	return checker.GetConfiguration()
}

// Get health manager changes through channel.
func (h *HealthManager) Updates() <-chan HealthManagerState {
	return h.updateChan
}

func (h *HealthManager) UpdateEntry(entry *healthStatusEntry, healthCheckPassed bool) bool {
	h.countLock.RLock()
	defer h.countLock.RUnlock()
	return entry.UpdateHealthCheckStatus(healthCheckPassed, h.riseCount, h.fallCount)
}

// Applies resolver State (adding new entries and removing nonexistent)
func (h *HealthManager) applyResolverState(discoveryState discovery.DiscoveryState) {
	state := h.state

	var newState HealthManagerState = make(
		[]HealthManagerEntry, len(discoveryState))

	for i, hostPort := range discoveryState {

		if h.params.DnsResolver != nil && h.params.AddressFamily >= 0 {
			ip, err := h.params.DnsResolver.ResolveHost(hostPort.Host, h.params.AddressFamily)
			if err == nil {
				// all healthcheckers are using JoinHostPort which is adding additional []
				// for v6
				hostPort.Address = strings.Trim(common.KglbAddrToString(ip), "[]")
			}
		}
		entry := h.state.GetEntry(hostPort)
		if entry != nil {
			newState[i] = *entry
		} else {
			newState[i] = HealthManagerEntry{
				HostPort: hostPort,
				Status:   NewHealthStatusEntry(h.params.InitialHealthyState),
				Enabled:  hostPort.Enabled,
			}
		}
	}

	// saving new state.
	dlog.Infof(
		"Health manager state has been updated: %s, %s -> %s",
		h.GetId(),
		h.state.String(),
		newState.String())
	h.state = newState

	h.initialResolverStateRecv = true

	// Notifying about the change in the state. Skipping it for initial state
	// which wasn't healthchecked yet.
	if !state.Equal(newState) && h.initialStateSent {
		h.notifyStateChange()
	}
}

// Internal loop to process updates from different sources like
// resolver, health checker.
func (h *HealthManager) healthCheckLoop() {
	// schedule next healthcheck
	nextHealthCheck := time.After(0)
	for {
		select {
		// resolver's updates.
		case resolverState, ok := <-h.resolver.Load().(discovery.DiscoveryResolver).Updates():
			if !ok {
				// resolver has been closed, nothing to health check without it.
				dlog.Infof("Closing Health Manager because of closed resolver: %s", h.GetId())
				// canceling context.
				h.Close()
				// update channel is not needed anymore.
				close(h.updateChan)
				return
			}
			// applying new state and notify about the change.
			h.applyResolverState(resolverState)
		case <-h.ctx.Done(): // stop chan.
			// update channel is not needed anymore.
			close(h.updateChan)
			dlog.Infof("Closing Health Manager: %s", h.GetId())
			return
		case <-nextHealthCheck: // health checking loop.
			nextHealthCheck = time.After(h.interval.Load().(time.Duration))

			if len(h.state) == 0 && !h.initialResolverStateRecv {
				dlog.Infof("Skipping healthcheck cycle as resolver state is not ready yet")
				continue
			}

			startTs := time.Now()
			changed := h.performHealthChecks()
			checkDuration := time.Since(startTs)

			// TODO(dkopytkov): report metric to properly monitor the value and
			//  adjust ConcurrencyLimit in the configs if it's needed.
			dlog.Infof("%s health manager took %v to check %d items",
				h.GetId(),
				checkDuration,
				len(h.state))

			if changed || !h.initialStateSent {
				h.notifyStateChange()
				// no more bypases of changed flag.
				h.initialStateSent = true
			}
		case <-h.updateConfChan: // configuration has been updated.
		}
	}
}

// Notifies about the change in the state through channel.
func (h *HealthManager) notifyStateChange() {
	// no need extra lock during Clone() call since performHealthChecks and
	// notifyStateChange is happening in the same goroutine (healthCheckLoop).
	stateToSend := h.state.Clone()

	healthyCnt := 0

	for _, entry := range stateToSend {
		if entry.Status.IsHealthy() {
			healthyCnt++
		}
	}

	aliveRatio := float64(0)
	if len(stateToSend) > 0 {
		aliveRatio = float64(healthyCnt) / float64(len(stateToSend))
	}

	h.setAliveRatioGauge(aliveRatio)

	h.lastUpdateState.Store(stateToSend)

	// remove state from chan if any
	select {
	case <-h.updateChan:
	default:
	}

	dlog.Infof("Sending state update by %s health manager: %s ", h.GetId(), stateToSend.String())

	// send a new state
	h.updateChan <- stateToSend
}

// Perform health checking of all entries. The call is blocking until
// all entries have been checked or context is canceled.
// TODO(oleg) upper-bound the performHealthChecks execution time to avoid
//  goroutine leakage
func (h *HealthManager) performHealthChecks() bool {
	numWorkers := int(atomic.LoadUint32(&h.concurrencyLimit))
	checker := h.checker.Load().(health_checker.HealthChecker)

	h.countLock.RLock()
	defer h.countLock.RUnlock()

	changed := uint32(0) // indicates change at least in single HostPort entry.
	err := concurrency.CompleteTasks(
		h.ctx,
		numWorkers,
		len(h.state),
		func(numWorker int, numTask int) {
			checkStatus := true
			enabled := h.state[numTask].Enabled
			// 1. perform check.
			if enabled {
				err := checker.Check(
					h.state[numTask].HostPort.Address,
					h.state[numTask].HostPort.Port)
				if err != nil {
					checkStatus = false
					// report about the issue.
					exclog.Report(
						errors.Wrapf(
							err,
							"%s health manager failed to check %s entry: ",
							h.params.Id,
							h.state[numTask].HostPort.String()),
						exclog.Operational, "")
				}
			} else {
				// do not run healthcheck (report host as down unconditionally) if host is marked
				// as disabled by Discovery service
				checkStatus = false
			}
			// 2. update status.
			if h.state[numTask].Status.UpdateHealthCheckStatus(checkStatus, h.riseCount, h.fallCount) {
				dlog.Infof(
					"%s health manager updated %s entry status to %v",
					h.params.Id,
					h.state[numTask].HostPort,
					h.state[numTask].Status.IsHealthy())
				atomic.StoreUint32(&changed, 1)
			}
			// 3. update realserver health status stats
			if checkStatus {
				h.increasePassCounter(h.state[numTask].HostPort.Host)
			} else if enabled {
				h.increaseFailCounter(h.state[numTask].HostPort.Host)
			}
		})
	//TODO(oleg) emit stats
	if err != nil {
		exclog.Report(err, exclog.Operational, "")
	}
	return atomic.LoadUint32(&changed) == 1
}

func (h *HealthManager) increasePassCounter(host string) {
	// Get saved / create new v2 counter for given host
	counter := h.getHealthCheckCounter(host, "pass", h.passCounters)

	if counter != nil {
		counter.Add(1)
	}
}

func (h *HealthManager) increaseFailCounter(host string) {
	// Get saved / create new v2 counter for given host
	counter := h.getHealthCheckCounter(host, "fail", h.failCounters)

	if counter != nil {
		counter.Add(1)
	}
}

func (h *HealthManager) setAliveRatioGauge(value float64) {
	h.statLock.Lock()
	defer h.statLock.Unlock()
	h.aliveGauge.Set(value)
}

func (h *HealthManager) getHealthCheckCounter(host, result string, srcMap statCounterMap) *v2stats.Counter {
	h.statLock.Lock()
	defer h.statLock.Unlock()

	if counter, ok := srcMap[host]; ok {
		return counter
	}

	newCounter, err := healthCheckCounter.V(v2stats.KV{
		"setup":   h.params.SetupName,
		"service": h.params.ServiceName,
		"host":    host,
		"result":  result,
	})
	if err != nil {
		exclog.Report(
			errors.Wrapf(err,
				"Failed to instantiate v2 healthcheck counter for setup %s, service %s",
				h.params.SetupName,
				h.params.ServiceName,
			),
			exclog.Critical, "",
		)
		return nil
	}

	srcMap[host] = &newCounter
	return &newCounter
}
