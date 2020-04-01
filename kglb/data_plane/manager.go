package data_plane

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"dropbox/dlog"
	"dropbox/exclog"
	"dropbox/kglb/common"
	"dropbox/kglb/utils/fwmark"
	kglb_pb "dropbox/proto/kglb"
	"dropbox/vortex2/v2stats"
	"godropbox/errors"
)

var (
	notImplErr = errors.New("not implemented")
)

// Custom shutdown handler to perform shutdown logic differently than
// it's done by default where data_plane cleans up the state.
type ShutdownHandlerFunc func(currentState *kglb_pb.DataPlaneState) error

// list of non-null modules required by Manager.
type ManagerModules struct {
	Bgp          BgpModule
	AddressTable AddressTableModule // netlink/addresses.
	Ipvs         IpvsModule

	// DNS related module.
	Resolver ResolverModule
	// Custom shutdown handler. Default handler will be used when it's not
	// specified.
	ShutdownHandler ShutdownHandlerFunc

	// v2 manager state gauge
	ManagerStateStat *v2stats.GaugeGroup

	// v2 manager state age
	// NOTE: defined as a pointer, so we can pass it from the dbx_data_plane
	ManagerStateAgeSec *v2stats.Gauge
}

type Manager struct {
	modules *ManagerModules
	mutex   sync.Mutex

	dynRoutingMng   *DynamicRoutingManager
	balancerManager *BalancerManager
	addressManager  *AddressManager

	shutdownHandler ShutdownHandlerFunc

	// v2 stats for service/upstream: [name] -> metrics
	serviceStats  map[string]*commonStats
	upstreamStats map[string]*commonStats

	// stats snapshot, map[ipvs_service][ipvs_real]*kglb_pb.Stats
	prevStats map[string]map[string]*kglb_pb.Stats

	lastSuccessfulStateChange time.Time

	shutdownOnce bool
}

type commonStats struct {
	bytesIn, bytesOut     v2stats.Counter
	packetsIn, packetsOut v2stats.Counter
}

func NewManager(params ManagerModules) (*Manager, error) {

	if params.ManagerStateStat == nil {
		// Currently for unittests only.
		// For real run gauge being created in dbx_data_plane
		params.ManagerStateStat = v2stats.NewGaugeGroup(ManagerStateGauge)
	}
	if params.ManagerStateAgeSec == nil {
		stat := ManagerStateAgeSec.Must()
		params.ManagerStateAgeSec = &stat
	}

	balancerMgn, err := NewBalancerManager(BalancerManagerParams{
		Ipvs:     params.Ipvs,
		Resolver: params.Resolver,
	})
	if err != nil {
		return nil, err
	}

	dynRoutingMng, err := NewDynamicRoutingManager(DynamicRoutingManagerParams{
		Bgp: params.Bgp,
	})
	if err != nil {
		return nil, err
	}

	addressManager, err := NewAddressManager(AddressManagerParams{
		AddressTable: params.AddressTable,
	})
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		modules:                   &params,
		balancerManager:           balancerMgn,
		dynRoutingMng:             dynRoutingMng,
		addressManager:            addressManager,
		prevStats:                 make(map[string]map[string]*kglb_pb.Stats),
		shutdownHandler:           params.ShutdownHandler,
		serviceStats:              make(map[string]*commonStats),
		upstreamStats:             make(map[string]*commonStats),
		lastSuccessfulStateChange: time.Now(),
	}

	// use default shutdown handler when custom is not specified.
	if manager.shutdownHandler == nil {
		dlog.Info("using default shutdown handler.")
		manager.shutdownHandler = manager.defaultShutdownHandler
	} else {
		dlog.Info("using custom shutdown handler.")
	}

	return manager, nil
}

// default shutdown handler.
func (m *Manager) defaultShutdownHandler(
	currentState *kglb_pb.DataPlaneState) error {

	// Withdrawing dynamic routes.
	if err := m.dynRoutingMng.WithdrawRoutes(currentState.GetDynamicRoutes()); err != nil {
		return err
	}

	// Removing balancers.
	if err := m.balancerManager.DeleteBalancers(currentState.GetBalancers()); err != nil {
		return err
	}

	// Removing ip address of the service.
	if err := m.addressManager.DeleteAddresses(currentState.GetLinkAddresses()); err != nil {
		return err
	}

	return nil
}

// Shutdown manager.
func (m *Manager) Shutdown() (err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	defer func() {
		if err != nil {
			m.emitManagerState("shutdown_failed")
		}
	}()

	if m.shutdownOnce {
		dlog.Info("Shutdown skipped, because it was already done.")
		return nil
	}

	m.shutdownOnce = true
	dlog.Infof("Shutdown Manager...")

	// Querying existent state first.
	currentState, err := m.getStateNonThreadSafe()
	if err != nil {
		return err
	}
	if err = m.shutdownHandler(currentState); err != nil {
		return err
	}

	dlog.Infof("Manager was successfully closed.")
	return nil
}

func (m *Manager) SetState(state *kglb_pb.DataPlaneState) (err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	defer func() {
		if err != nil {
			m.emitManagerState("set_state_failed")
		} else {
			m.lastSuccessfulStateChange = time.Now()
			m.emitManagerState("available")
		}
	}()

	if m.shutdownOnce {
		return errors.New("state cannot by applied after shutdown.")
	}

	// update cache to get state with proper hostnames.
	if cacheResolver, ok := m.modules.Resolver.(*CacheResolver); ok {
		// 1. update resolver cache.
		cacheResolver.UpdateCache(state)
	}

	// 1. Querying existent state first.
	currentState, err := m.getStateNonThreadSafe()
	if err != nil {
		return err
	}

	// 2. Identifying difference in balancers and bgp announcements.
	balancersDiff := common.CompareBalancerState(
		currentState.GetBalancers(),
		state.GetBalancers())

	routingDiff := common.CompareDynamicRouting(
		currentState.GetDynamicRoutes(),
		state.GetDynamicRoutes())

	localAddressessDiff := common.CompareLocalLinkAddresses(
		currentState.GetLinkAddresses(),
		state.GetLinkAddresses())

	// check if there is any changes.
	if !balancersDiff.IsChanged() &&
		!routingDiff.IsChanged() &&
		!localAddressessDiff.IsChanged() {

		dlog.Infof("data plane state is not changed.")
		return nil
	}

	dlog.Infof("Applying new state...")

	// 3. Applying changes in the right order:
	// a) remove deleted bgp routes.
	// c) adding new balancers.
	// d) adding ip address of the service.
	// e) update existent balancers (their upstreams).
	// f) advertise bgp routes.
	// g) remove deleted balancer.
	// h) deleted addresses related to deleted balancers.

	if len(routingDiff.Deleted) > 0 {
		dlog.Infof("a) Withdrawing set of routes: %+v", routingDiff.Deleted)
		err = m.dynRoutingMng.WithdrawRoutes(
			common.DynamicRoutingConvBack(routingDiff.Deleted))
		if err != nil {
			return errors.Wrap(err, "fails to withdraw routing path: ")
		}
	}

	// c) adding new balancers.
	if len(balancersDiff.Added) > 0 {
		dlog.Infof("c) Adding balancers: %+v", balancersDiff.Added)
		err = m.balancerManager.AddBalancers(
			common.BalancerStateConvBack(balancersDiff.Added))
		if err != nil {
			return errors.Wrap(err, "fails to add balancer: ")
		}
	}

	// d) adding ip address of the service.
	if len(localAddressessDiff.Added) > 0 {
		dlog.Infof("d) Adding addresses: %+v", localAddressessDiff.Added)
		err = m.addressManager.AddAddresses(
			common.LinkAddressStateConvBack(localAddressessDiff.Added))
		if err != nil {
			return errors.Wrap(err, "fails to add addresses: ")
		}
	}

	// e) update existent balancers (updating reals in our case).
	for _, pair := range balancersDiff.Changed {

		upstreamDiff := common.CompareUpstreamState(
			pair.OldItem.(*kglb_pb.BalancerState).GetUpstreams(),
			pair.NewItem.(*kglb_pb.BalancerState).GetUpstreams())

		lbService := pair.NewItem.(*kglb_pb.BalancerState).GetLbService()
		if upstreamDiff.IsChanged() {
			// adding upstreams.
			if len(upstreamDiff.Added) > 0 {
				dlog.Infof("e) Adding upstreams for balancer: %s :%+v",
					pair.NewItem.(*kglb_pb.BalancerState).GetName(),
					upstreamDiff.Added)
				err = m.balancerManager.AddUpstreams(
					lbService,
					common.UpstreamStateConvBack(upstreamDiff.Added))
				if err != nil {
					return errors.Wrapf(
						err, "fails to add upstreams: %+v, error: ", lbService)
				}
			}
			// updating upstream.
			if len(upstreamDiff.NewChangedStates()) > 0 {
				dlog.Infof("e) Updating upstreams for balancer: %s :%+v",
					pair.NewItem.(*kglb_pb.BalancerState).GetName(),
					upstreamDiff.NewChangedStates())
				err = m.balancerManager.UpdateUpstreams(
					lbService,
					common.UpstreamStateConvBack(upstreamDiff.NewChangedStates()))
				if err != nil {
					return errors.Wrapf(err, "fails to update upstreams: %+v, error: ", lbService)
				}
			}
			// deleting upstream.
			if len(upstreamDiff.Deleted) > 0 {
				dlog.Infof("e) Deleting upstreams for balancer: %s :%+v",
					pair.NewItem.(*kglb_pb.BalancerState).GetName(),
					upstreamDiff.Deleted)
				err = m.balancerManager.DeleteUpstreams(
					lbService,
					common.UpstreamStateConvBack(upstreamDiff.Deleted))
				if err != nil {
					return errors.Wrapf(err, "fails to delete upstreams: %+v, error: ", lbService)
				}
			}
		}
	}

	// f) advertise bgp routes.
	if len(routingDiff.Added) > 0 {
		dlog.Infof("f) Advertise routes: %+v", routingDiff.Added)
		err = m.dynRoutingMng.AdvertiseRoutes(
			common.DynamicRoutingConvBack(routingDiff.Added))
		if err != nil {
			return errors.Wrap(err, "fails to advertise routing path: ")
		}
	}

	// g) remove deleted balancers.
	if len(balancersDiff.Deleted) > 0 {
		dlog.Infof("g) Deleting balancers: %+v", balancersDiff.Deleted)
		err = m.balancerManager.DeleteBalancers(
			common.BalancerStateConvBack(balancersDiff.Deleted))
		if err != nil {
			return errors.Wrap(err, "fails to delete balancers: ")
		}
	}

	// h) removing ip address of the service.
	if len(localAddressessDiff.Deleted) > 0 {
		dlog.Infof("h) Deleting addresses: %+v", localAddressessDiff.Deleted)
		err = m.addressManager.DeleteAddresses(
			common.LinkAddressStateConvBack(localAddressessDiff.Deleted))
		if err != nil {
			return errors.Wrap(err, "fails to delete addresses: ")
		}
	}

	dlog.Infof("New state has been successfully applied.")
	return nil
}

func (m *Manager) getStateNonThreadSafe() (state *kglb_pb.DataPlaneState, err error) {
	balancerState, err := m.balancerManager.GetBalancers()
	if err != nil {
		return nil, errors.Wrap(err, "BalancerManager error: ")
	}

	routingState, err := m.dynRoutingMng.ListRoutes()
	if err != nil {
		return nil, errors.Wrap(err, "DynamicRoutingManager error: ")
	}

	linkAddresses, err := m.addressManager.State()
	if err != nil {
		return nil, errors.Wrap(err, "AddressManager error: ")
	}

	return &kglb_pb.DataPlaneState{
		Balancers:     balancerState,
		DynamicRoutes: routingState,
		LinkAddresses: linkAddresses,
	}, nil
}

// Get current manager state.
func (m *Manager) GetState() (*kglb_pb.DataPlaneState, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.getStateNonThreadSafe()
}

// Collect real data from services and emit them into stats.
func (m *Manager) EmitStats() error {
	diffSec := time.Since(m.lastSuccessfulStateChange).Seconds()
	m.modules.ManagerStateAgeSec.Set(diffSec)

	// get all ipvs services.
	services, servicesStats, err := m.modules.Ipvs.ListServices()
	if err != nil {
		return errors.Wrap(err, "fails to get services: ")
	}

	// emit ipvs service and upstream stats
	for i, ipvsService := range services {
		if fwmark.IsFwmarkService(ipvsService) {
			// we are using fwmark services only for healthchecks. ignore em and
			// do not generate any vortex2 stats
			continue
		}
		serviceStats := servicesStats[i]

		// check previous stored stats and emit metrics if there is data
		prevServiceStats, ok := m.prevStats[ipvsService.String()]
		if ok {
			name := strings.Replace(m.modules.Resolver.ServiceLookup(ipvsService), ":", "_", -1)
			err := m.emitServiceStats(name, prevServiceStats["service"], serviceStats)
			if err != nil {
				exclog.Report(
					errors.Wrap(err, "fails to emit stats: "), exclog.Critical, "")
			}
		} else {
			m.prevStats[ipvsService.String()] = map[string]*kglb_pb.Stats{}
		}
		m.prevStats[ipvsService.String()]["service"] = servicesStats[i]

		realServers, realsStats, err := m.modules.Ipvs.GetRealServers(ipvsService)
		if err != nil {
			exclog.Report(
				errors.Wrap(err, "fails to get real servers: "), exclog.Noncritical, "")
			// continue to emit what is possible.
			continue
		}

		// emit upstream specific stats.
		for j, realServer := range realServers {
			realStats := realsStats[j]
			// IPVS sets ForwardMethod 'Masq' to for 0-weighted backends,
			// so use IP:PORT as an unique key
			realServerKey := fmt.Sprintf("%s:%d",
				common.KglbAddrToNetIp(realServer.Address), realServer.Port)

			prevUpstreamStats, ok := prevServiceStats[realServerKey]
			if ok {
				err := m.emitUpstreamStats(getUpstreamHostname(realServer), prevUpstreamStats, realStats)
				if err != nil {
					exclog.Report(
						errors.Wrap(err, "fails to emit stats: "), exclog.Critical, "")
				}
			}
			m.prevStats[ipvsService.String()][realServerKey] = realStats
		}
	}

	return nil
}

func (m *Manager) emitServiceStats(name string, prev, current *kglb_pb.Stats) error {
	// Instantiate v2 counters/gauge if needed.
	stats, exists := m.serviceStats[name]
	if !exists {
		var err error
		stats, err = instantiateCommonStats(name, serviceBytesCounter,
			servicePacketsCounter)
		if err != nil {
			return err
		}
		m.serviceStats[name] = stats
	}

	emitCommonMetrics(stats, prev, current)

	return nil
}

func (m *Manager) emitUpstreamStats(name string, prev, current *kglb_pb.Stats) error {
	// Instantiate v2 counters/gauge if needed.
	stats, exists := m.upstreamStats[name]
	if !exists {
		var err error
		stats, err = instantiateCommonStats(name, upstreamBytesCounter,
			upstreamPacketsCounter)
		if err != nil {
			return err
		}
		m.upstreamStats[name] = stats
	}

	emitCommonMetrics(stats, prev, current)

	return nil
}

func (m *Manager) emitManagerState(state string) {
	if err := m.modules.ManagerStateStat.PrepareToSet(1, v2stats.KV{"state": state}); err != nil {
		exclog.Report(
			errors.Wrap(err, "unable to PrepareToSet() managerState gauge"), exclog.Critical, "")
	}
	m.modules.ManagerStateStat.SetAndReset()
}

func instantiateCommonStats(name string,
	byteCounter, packetCounter v2stats.CounterDefinition) (*commonStats, error) {

	var err error
	stats := &commonStats{}
	stats.bytesIn, err = byteCounter.V(v2stats.KV{
		"name":      name,
		"direction": "in",
	})
	if err != nil {
		return nil, err
	}
	stats.bytesOut, err = byteCounter.V(v2stats.KV{
		"name":      name,
		"direction": "out",
	})
	if err != nil {
		return nil, err
	}
	stats.packetsIn, err = packetCounter.V(v2stats.KV{
		"name":      name,
		"direction": "in",
	})
	if err != nil {
		return nil, err
	}
	stats.packetsOut, err = packetCounter.V(v2stats.KV{
		"name":      name,
		"direction": "out",
	})
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func emitCommonMetrics(stats *commonStats, prev, current *kglb_pb.Stats) {
	stats.bytesIn.Add(diffMetric(current.BytesInCount, prev.BytesInCount))
	stats.bytesOut.Add(diffMetric(current.BytesOutCount, prev.BytesOutCount))
	stats.packetsIn.Add(diffMetric(current.PacketsInCount, prev.PacketsInCount))
	stats.packetsOut.Add(diffMetric(current.PacketsOutCount, prev.PacketsOutCount))
}
