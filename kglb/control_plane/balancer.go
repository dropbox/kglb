package control_plane

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"

	"dropbox/dlog"
	"dropbox/exclog"
	"dropbox/kglb/common"
	"dropbox/kglb/utils/discovery"
	"dropbox/kglb/utils/dns_resolver"
	"dropbox/kglb/utils/fwmark"
	"dropbox/kglb/utils/health_manager"
	pb "dropbox/proto/kglb"
	"dropbox/vortex2/v2stats"
	"godropbox/errors"
)

const (
	DefaultWeightUp   = uint32(1000)
	DefaultWeightDown = uint32(0)
	// default number of concurrent health checks.
	DefaultCheckerConcurrency = 100
	// default delay between updating state retry attempts in case of failed dns
	// resolution.
	DefaultUpdateRetryWaitTime = 5 * time.Second
)

type BalancerParams struct {
	// Upstream configuration.
	BalancerConfig *pb.BalancerConfig

	// Discovery Resolver factory.
	ResolverFactory DiscoveryFactory
	// Health Checker factory.
	CheckerFactory HealthCheckerFactory
	// Dns Resolver.
	DnsResolver dns_resolver.DnsResolver

	// Channel where balancer will send updated states.
	UpdatesChan chan *BalancerState

	// global fwmark manager
	FwmarkManager *fwmark.Manager

	// wait time between updating state retry attempts in case of failed
	// dns resolution.
	UpdateRetryWaitTime time.Duration
}

// Discovers, health checks, resolves hostnames and generates []*pb.BalancerState
// during changes. Balancer provides updates of the state via UpdatesChan
// channel provided in BalancerParams, it doesn't create own to simplify
// monitoring changes across multiple Balancers.
type Balancer struct {
	mutex sync.Mutex

	// Upstream name.
	name string

	// VIP of the balancer.
	vip string

	// Health Manager.
	healthMng *health_manager.HealthManager

	// Fwmark Manager
	fwmarkMng *fwmark.Manager

	// Discovery Resolver.
	resolver discovery.DiscoveryResolver

	// Config update chan.
	updatesConf chan struct{}

	// latest successfully applied balancer config.
	config *pb.BalancerConfig
	params *BalancerParams

	// latest balancer state.
	state        atomic.Value // *BalancerState
	initialState bool

	// v2 stats
	statUpstreamsCount *v2stats.GaugeGroup
	statBalancerState  *v2stats.GaugeGroup

	ctx        context.Context
	cancelFunc context.CancelFunc

	// Weights of healthy reals.
	weightUp uint32
}

func getAddressFamilyFromVip(vip string) pb.AddressFamily {
	ip := net.ParseIP(vip)
	if ip == nil {
		// fwmark vip
		return -1
	}
	if ip.To4() != nil {
		return pb.AddressFamily_AF_INET
	}
	return pb.AddressFamily_AF_INET6
}

// for testing.
func newBalancer(
	ctx context.Context,
	params BalancerParams) (*Balancer, error) {

	// use default timeout for updating state retry attempts wait time when
	// it's not specified.
	if params.UpdateRetryWaitTime == 0 {
		params.UpdateRetryWaitTime = DefaultUpdateRetryWaitTime
	}

	// Balancer instance.
	up := &Balancer{
		name:         params.BalancerConfig.GetName(),
		config:       params.BalancerConfig,
		updatesConf:  make(chan struct{}, 1),
		params:       &params,
		fwmarkMng:    params.FwmarkManager,
		initialState: true,
		weightUp:     params.BalancerConfig.GetWeightUp(),
		// v2 stats
		statBalancerState:  v2stats.NewGaugeGroup(balancerStateGauge),
		statUpstreamsCount: v2stats.NewGaugeGroup(upstreamsCountGauge),
	}

	if up.weightUp == 0 {
		up.weightUp = DefaultWeightUp
	}

	up.ctx, up.cancelFunc = context.WithCancel(ctx)

	// extracting vip.
	vip, _, err := common.GetVipFromLbService(params.BalancerConfig.GetLbService())
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"fails to extract vip: %+v: ",
			params.BalancerConfig)
	}
	up.vip = vip

	up.params = &params

	// 1. Creating discovery resolver.
	discoveryConf := up.config.GetUpstreamDiscovery()
	up.resolver, err = params.ResolverFactory.Resolver(
		up.name,
		up.params.BalancerConfig.SetupName,
		discoveryConf)
	if err != nil {
		return nil, errors.Wrapf(err, "fails to create resolver: ")
	}

	// 2. Creating HealthChecker.
	healthchecker, err := up.params.CheckerFactory.Checker(up.config)
	if err != nil {
		return nil, errors.Wrapf(err, "fails to create checker: ")
	}

	healthManagerParams := health_manager.HealthManagerParams{
		Id:                        up.Name(),
		Resolver:                  up.resolver,
		DnsResolver:               params.DnsResolver,
		HealthChecker:             healthchecker,
		SetupName:                 up.params.BalancerConfig.SetupName,
		ServiceName:               up.Name(),
		AddressFamily:             getAddressFamilyFromVip(vip),
		UpstreamCheckerAttributes: up.config.GetUpstreamChecker(),
	}

	up.healthMng, err = health_manager.NewHealthManager(up.ctx, healthManagerParams)
	if err != nil {
		return nil, errors.Wrapf(err, "fails to create health manager: ")
	}

	// balancer is in initial state since there was no any healthy upstreams.
	up.state.Store(&BalancerState{
		InitialState: true,
	})

	return up, nil
}

func NewBalancer(
	ctx context.Context,
	params BalancerParams) (*Balancer, error) {

	up, err := newBalancer(ctx, params)
	if err != nil {
		return nil, err
	}

	up.updateBalancerStateGauge(1, "initial")
	// starting update loop.
	go up.updateLoop()

	return up, nil
}

// Returns Balancer name.
func (u *Balancer) Name() string {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	return u.name
}

// Returns latest successfully applied config.
func (u *Balancer) GetConfig() *pb.BalancerConfig {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	return u.config
}

// Returns current state of the Balancer.
func (u *Balancer) GetState() *BalancerState {
	return u.state.Load().(*BalancerState)
}

// Returns channel provided in params. Balancer sends new states into it when
// it's writable.
func (u *Balancer) Updates() <-chan *BalancerState {
	return u.params.UpdatesChan
}

// Updates balancer.
// FIXME(oleg) Update() should never be used as with current implementation
//  there is high probability of leaving Balancer in inconsistent state
//  with partially applied changes.
//  Instead Balancer should be recreated and swapped.
func (u *Balancer) Update(conf *pb.BalancerConfig) error {
	// process single update at a time.
	u.mutex.Lock()
	defer u.mutex.Unlock()

	// Going through BalancerConfig and update internals.
	// 1. Sanity check (name and setup name in the config should match existent).
	if u.name != conf.GetName() {
		return errors.Newf("Updating balancer name is not allowed: %s -> %s",
			u.name, conf.GetName())
	}
	if u.config.GetSetupName() != conf.GetSetupName() {
		return errors.Newf("Updating balancer setup name is not allowed: %s -> %s",
			u.config.GetSetupName(), conf.GetSetupName())
	}
	// checking vip.
	vip, _, err := common.GetVipFromLbService(conf.GetLbService())
	if err != nil {
		return errors.Wrapf(err, "fails to extract vip: %+v: ", conf)
	}
	// changing vip is not allowed because it might affect stats.
	if u.vip != vip {
		return errors.Newf(
			"Updating vip is not allowed: %s, %s -> %s",
			u.name,
			u.vip,
			vip)
	}

	// 2. Updating discovery.
	discoveryConf := conf.GetUpstreamDiscovery()
	err = u.params.ResolverFactory.Update(u.resolver, discoveryConf)
	if err == ErrResolverIncompatibleType {
		resolver, err := u.params.ResolverFactory.Resolver(
			u.name,
			conf.SetupName,
			discoveryConf)
		if err != nil {
			return errors.Wrapf(err, "fails to create resolver: ")
		}
		// update resolver.
		u.healthMng.UpdateResolver(resolver)
		// closing old resolver and updating reference.
		u.resolver.Close()
		u.resolver = resolver
	} else if err != nil {
		return errors.Wrapf(err, "fails to update resolver: ")
	}

	checkerConf := conf.GetUpstreamChecker()

	// 3. Updating HealthChecker if configuration changed.
	if !proto.Equal(u.healthMng.GetCheckerConfiguration(), checkerConf.GetChecker()) {
		dlog.Info("creating new upstream checker: %+v", checkerConf)
		checker, err := u.params.CheckerFactory.Checker(conf)
		if err != nil {
			return errors.Wrapf(err, "fails to create health checker: ")
		}
		// updating health manager.
		u.healthMng.UpdateHealthChecker(checker)
	}

	// 4. Updating health thresholds.
	if err := u.healthMng.Update(checkerConf); err != nil {
		return errors.Wrapf(err, "failed to update health manager")
	}

	// 6. Update reference to the config.
	u.config = conf

	// updating healthy weight.
	newWeightUp := conf.GetWeightUp()
	if newWeightUp == 0 {
		newWeightUp = DefaultWeightUp
	}

	if u.weightUp != newWeightUp {
		u.weightUp = newWeightUp

		// force regeneration balancer state, otherwise it will require update
		// from health manager.
		select {
		case u.updatesConf <- struct{}{}:
		default:
		}
	}

	return nil
}

// Closes Balancer.
func (u *Balancer) Close() {
	// "Reset" balancer state gauge
	u.updateBalancerStateGauge(0, "shutdown")
	// canceling context which will close manager and update channel.
	u.cancelFunc()
	u.resolver.Close()
}

func getAddressString(state *pb.UpstreamState) string {
	ip := state.GetAddress()
	switch ip.GetAddress().(type) {
	case *pb.IP_Ipv4:
		return ip.GetIpv4()
	case *pb.IP_Ipv6:
		return ip.GetIpv6()
	default:
		return ""
	}
}

func (u *Balancer) manageFwmarkAllocations(upstreamStates []*pb.UpstreamState) {
	// what have added:
	balancerStates := u.GetState()
	for _, state := range upstreamStates {
		addr := getAddressString(state)
		found := false
		if len(balancerStates.States) > 0 {
			for _, oldState := range balancerStates.States[0].Upstreams {

				if addr == getAddressString(oldState) {
					found = true
					break
				}
			}
		}
		if !found {
			// new upstream. we need to allocate new fwmark for it
			_, err := u.fwmarkMng.AllocateFwmark(addr)
			if err != nil {
				exclog.Report(
					errors.Newf("failed to allocate fwmark for: %s error: %s", state.Hostname, err.Error()),
					exclog.Critical, "")
			}
		}
	}
	if len(balancerStates.States) > 0 {
		// what was removed:
		for _, oldState := range balancerStates.States[0].Upstreams {
			addr := getAddressString(oldState)
			found := false
			for _, state := range upstreamStates {
				if addr == getAddressString(state) {
					found = true
					break
				}
			}
			if !found {
				// hostname not in the list of backends anymore. release fwmark
				err := u.fwmarkMng.ReleaseFwmark(addr)
				if err != nil {
					exclog.Report(
						errors.Newf("failed to release fwmark for: %s error: %s", oldState.Hostname, err.Error()),
						exclog.Critical, "")
				}
			}
		}
	}
}

func (u *Balancer) generateFwmarkService(
	vip string,
	upstreamState *pb.UpstreamState) *pb.BalancerState {

	fwmarkVal, err := u.fwmarkMng.GetAllocatedFwmark(getAddressString(upstreamState))

	if err != nil {
		exclog.Report(
			errors.Newf("failed to get allocated fwmark for: %s error: %s",
				upstreamState.Hostname, err.Error()),
			exclog.Critical, "")
		return nil
	}

	return &pb.BalancerState{
		Name: fmt.Sprintf("fwmark%d", fwmarkVal),
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_FwmarkAttributes{
						FwmarkAttributes: &pb.IpvsFwmarkAttributes{
							Fwmark:        fwmarkVal,
							AddressFamily: common.KglbAddrToFamily(upstreamState.GetAddress()),
						},
					},
				},
			},
		},
		Upstreams: []*pb.UpstreamState{
			{
				Hostname: upstreamState.GetHostname(),
				Port:     uint32(0),
				Address:  upstreamState.GetAddress(),
				// mark as UP, otherwise ipvs drops health checks
				Weight:        u.weightUp,
				ForwardMethod: upstreamState.GetForwardMethod(),
			},
		},
	}
}

// Updates manager's state.
func (u *Balancer) updateState(
	state health_manager.HealthManagerState) {

	u.mutex.Lock()
	defer u.mutex.Unlock()

	// generate UpstreamState based on provided HealthManagerState state.
	upstreamStates := []*pb.UpstreamState{}
	for _, entry := range state {
		// getting ip.
		ip, err := u.params.DnsResolver.ResolveHost(
			entry.HostPort.Host,
			u.config.GetUpstreamDiscovery().GetResolveFamily())
		if err != nil {
			exclog.Report(
				errors.Wrapf(
					err,
					"fails to resolve %s in %s balancer: ",
					entry.HostPort.Host,
					u.name),
				exclog.Critical, "")
			// try to regenerate state in few seconds, otherwise it will not be
			// happened until next update from health manager.
			time.AfterFunc(u.params.UpdateRetryWaitTime, func() {
				select {
				case u.updatesConf <- struct{}{}:
					dlog.Infof(
						"will retry to update state of %s balancer in %v because "+
							"of failed dns resolution.",
						u.name,
						u.params.UpdateRetryWaitTime)
				default:
				}
			})
			return
		}
		upstream := &pb.UpstreamState{
			Hostname: entry.HostPort.Host,
			Port:     uint32(entry.HostPort.Port),
			Address:  ip,
			// forward method.
			ForwardMethod: u.config.GetUpstreamRouting().GetForwardMethod(),
		}
		if entry.Status.IsHealthy() {
			upstream.Weight = u.weightUp
		} else {
			upstream.Weight = DefaultWeightDown
		}
		upstreamStates = append(upstreamStates, upstream)
	}

	// main state.
	aliveRatio := common.AliveUpstreamsRatio(upstreamStates)
	if u.initialState && aliveRatio > 0 {
		u.initialState = false
	}

	// TODO(belyalov): remove v1 stat
	upstreamCnt := len(upstreamStates)

	// doesn't make sense to announce route without any upstreams.
	if upstreamCnt == 0 {
		dlog.Info(
			"balancer doesn't have any upstreams: ",
			u.name)
	}

	// marking all backends as healthy when all of them failing healthcheck,
	// because it
	if !u.initialState && aliveRatio == 0 && upstreamCnt > 0 {
		dlog.Error("failsafe mode is enabled: ", u.name)
		u.updateBalancerStateGauge(1, "failsafe")
		for _, state := range upstreamStates {
			state.Weight = u.weightUp
		}
		// updating alive ratio since weight was modified.
		aliveRatio = common.AliveUpstreamsRatio(upstreamStates)
	} else {
		dlog.Info("failsafe mode is disabled: ", u.name)
		if upstreamCnt == 0 {
			u.updateBalancerStateGauge(1, "no_upstreams")
		} else {
			u.updateBalancerStateGauge(1, "available")
		}
	}

	// Report alive/dead upstream counters
	u.updateUpstreamCountGauge(upstreamStates)

	balancerState := &BalancerState{
		States: []*pb.BalancerState{
			{
				Name:      u.name,
				LbService: u.config.GetLbService(),
				Upstreams: upstreamStates,
			},
		},
		AliveRatio:   aliveRatio,
		InitialState: u.initialState,
	}

	// generate fwmark states.
	var fwmarkStates []*pb.BalancerState
	if u.config.GetEnableFwmarks() {
		u.manageFwmarkAllocations(upstreamStates)
		fwmarkStates = make([]*pb.BalancerState, len(upstreamStates))
		i := 0
		for _, upstreamState := range upstreamStates {
			service := u.generateFwmarkService(u.vip, upstreamState)
			if service != nil {
				fwmarkStates[i] = service
				i++
			}
		}

		balancerState.States = append(
			balancerState.States,
			fwmarkStates[0:i]...)
	}

	// update state.
	u.state.Store(balancerState)
	// notify about the change.
	select {
	case u.params.UpdatesChan <- balancerState:
		dlog.Infof(
			"new state has been sent by Balancer: %+v",
			balancerState)
	default:
		dlog.Infof(
			"updateChan of Balancer is full: %s",
			u.name)
	}
}

// update loop to handle updates from health manager.
func (u *Balancer) updateLoop() {
	for {
		select {
		case _, ok := <-u.healthMng.Updates():
			if !ok {
				exclog.Report(
					errors.Newf("health manager balancer has been closed: %s: ", u.name),
					exclog.Critical, "")
				u.Close()
				return
			}
			state := u.healthMng.GetState()
			dlog.Infof("performing update by Balancer: %s, %+v", u.name, state)
			u.updateState(state)
		case <-u.ctx.Done():
			dlog.Infof("Closing Balancer: %s", u.name)
			u.Close()
			return
		case <-u.updatesConf:
			// regenerate config because of Balancer change.
			u.updateState(u.healthMng.GetState())
		}
	}
}

// Updates current state of balancer
func (u *Balancer) updateBalancerStateGauge(value float64, state string) {
	tags := v2stats.KV{
		"setup":   u.params.BalancerConfig.SetupName,
		"service": u.params.BalancerConfig.Name,
		"state":   state,
	}
	err := u.statBalancerState.PrepareToSet(value, tags)
	if err != nil {
		exclog.Report(errors.Wrapf(err,
			"unable to set balancerState gauge (%v)", tags), exclog.Critical, "")
		return
	}

	u.statBalancerState.SetAndReset()
}

// Updates current upstreams counts (alive/dead) v2 gauge
func (u *Balancer) updateUpstreamCountGauge(upstreams []*pb.UpstreamState) {
	total := len(upstreams)
	alive := 0
	for _, u := range upstreams {
		if u.Weight != 0 {
			alive += 1
		}
	}

	tags := v2stats.KV{
		"setup":   u.params.BalancerConfig.SetupName,
		"service": u.params.BalancerConfig.Name,
		"alive":   "true",
	}

	err := u.statUpstreamsCount.PrepareToSet(float64(alive), tags)
	if err != nil {
		exclog.Report(errors.Wrapf(err,
			"unable to set upstreamsCountGauge gauge (%v)", tags), exclog.Critical, "")
		return
	}

	tags["alive"] = "false"
	err = u.statUpstreamsCount.PrepareToSet(float64(total-alive), tags)
	if err != nil {
		exclog.Report(errors.Wrapf(err,
			"unable to set upstreamsCountGauge gauge (%v)", tags), exclog.Critical, "")
		return
	}

	u.statUpstreamsCount.SetAndReset()
}
