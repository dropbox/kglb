package control_plane

import (
	"context"
	go_context "context"
	"fmt"
	"hash/fnv"
	"net"
	"sync"
	"time"

	"github.com/gogo/protobuf/types"

	"dropbox/dlog"
	"dropbox/exclog"
	"dropbox/kglb/common"
	common_config_loader "dropbox/kglb/utils/config_loader"
	"dropbox/kglb/utils/dns_resolver"
	"dropbox/kglb/utils/fwmark"
	pb "dropbox/proto/kglb"
	"dropbox/vortex2/v2stats"
	"godropbox/errors"
)

// Custom handler called by Control Plane after initialization which includes
// passing through following steps:
// 1. completion to parse and apply configuration.
// 2. completion to setup balancers.
// 3. completion to wait balancers states (completed one cycle of discovery and
// healthchecking reals).
// 4. successfully applied data plane state.
type AfterInitHandlerFunc func()

type ControlPlaneServicer struct {
	// mutext to protect state.
	mu sync.Mutex

	ctx        context.Context
	cancelFunc context.CancelFunc

	// protected by configLock mutex.
	config               *pb.ControlPlaneConfig
	balancersUpdatesChan chan *BalancerState

	balancers map[string]*Balancer
	state     *pb.DataPlaneState

	// v2 stats
	statAvailability      *v2stats.GaugeGroup
	statRouteAnnouncement *v2stats.GaugeGroup
	perSetupStateHashes   map[string]*v2stats.Gauge
	// modules provided during construction.
	modules ServicerModules

	initialState bool
}

// list of non-null modules required by Manager.
type ServicerModules struct {
	DiscoveryFactory DiscoveryFactory
	CheckerFactory   HealthCheckerFactory
	DataPlaneClient  DataPlaneClient
	DnsResolver      dns_resolver.DnsResolver
	ConfigLoader     common_config_loader.ConfigLoader

	// fwmark manager
	FwmarkManager *fwmark.Manager

	// handler called right after initialization.
	AfterInitHandler AfterInitHandlerFunc
}

// for testing purpose. Creates Servicer instance without starting
// startBackgroundPusher goroutine.
func newControlPlaneServicer(
	ctx context.Context,
	modules ServicerModules,
	processInterval time.Duration) (*ControlPlaneServicer, error) {

	servicer := &ControlPlaneServicer{
		modules:               modules,
		balancers:             make(map[string]*Balancer),
		balancersUpdatesChan:  make(chan *BalancerState, 1),
		initialState:          true,
		statAvailability:      v2stats.NewGaugeGroup(availabilityGauge),
		statRouteAnnouncement: v2stats.NewGaugeGroup(routeAnnouncementGauge),
		perSetupStateHashes:   make(map[string]*v2stats.Gauge),
	}
	servicer.ctx, servicer.cancelFunc = context.WithCancel(ctx)

	return servicer, nil
}

func NewControlPlaneServicer(
	ctx context.Context,
	modules ServicerModules,
	processInterval time.Duration) (*ControlPlaneServicer, error) {

	servicer, err := newControlPlaneServicer(ctx, modules, processInterval)
	if err != nil {
		return nil, err
	}

	servicer.updateAvailabilityGauge(0)

	// single goroutine to handle config updates and state changes.
	go servicer.startBackgroundPusher(processInterval)

	return servicer, nil
}

func (s *ControlPlaneServicer) GetConfiguration(
	ctx go_context.Context,
	req *types.Empty) (*pb.DataPlaneState, error) {

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.state, nil
}

func (s *ControlPlaneServicer) updateConfig(config *pb.ControlPlaneConfig) error {
	// 1. Create required balancer.
	for _, balancerConfig := range config.Balancers {
		balancerName := balancerConfig.GetName()
		// since name is used for stats only and it is fine to have multiple
		// balancer with the same name but different vip:vport then use
		// name and vip:vport as unique identification of balancer.
		key, err := common.GetKeyFromBalancerConfig(balancerConfig)
		if err != nil {
			exclog.Report(err, exclog.Critical, "")
			return err
		}
		if balancer, ok := s.balancers[key]; ok {
			// update existent balancer.
			dlog.Infof("Updating balancer: %s, %s", balancerName, key)
			err := balancer.Update(balancerConfig)
			if err != nil {
				exclog.Report(
					errors.Wrapf(err, "fails to update balancer: "),
					exclog.Critical, "")
				return err
			}
		} else {
			dlog.Infof("Adding balancer: %s, %s", balancerName, key)

			// create new balancer.
			balancerParams := BalancerParams{
				BalancerConfig:  balancerConfig,             // config
				ResolverFactory: s.modules.DiscoveryFactory, // resolver factory
				CheckerFactory:  s.modules.CheckerFactory,   // checker factory
				DnsResolver:     s.modules.DnsResolver,      // dns module
				UpdatesChan:     s.balancersUpdatesChan,     // updates channel
				FwmarkManager:   s.modules.FwmarkManager,
			}
			balancer, err = NewBalancer(s.ctx, balancerParams)
			if err != nil {
				exclog.Report(
					errors.Wrapf(err, "fails to create balancer: "),
					exclog.Critical, "")
				return err
			}
			s.balancers[key] = balancer
		}
	}

	// 2. Shutdown needless balancers.
	for balancerId, balancer := range s.balancers {
		found := false
		for _, balancerConfig := range config.Balancers {
			key, err := common.GetKeyFromBalancerConfig(balancerConfig)
			if err != nil {
				exclog.Report(err, exclog.Critical, "")
				return err
			}

			if balancerId == key {
				found = true
				break
			}
		}

		if !found {
			dlog.Infof("Removing balancer: %s, %s", balancer.Name(), balancerId)
			balancer.Close()
			delete(s.balancers, balancerId)
		}
	}

	s.config = config
	return nil
}

// Generates set of link addresses based on configuration and
// generated balancers.
func (s *ControlPlaneServicer) generateLinkAddrs() ([]*pb.LinkAddress, error) {
	// map[vip]*LinkAddress
	addrMap := make(map[string]*pb.LinkAddress)

	for _, balancer := range s.balancers {
		// iterate through all generate states to extract vips.
		balancerState := balancer.GetState()
		for _, state := range balancerState.States {
			var vip *pb.IP
			switch attr := state.GetLbService().GetIpvsService().Attributes.(type) {
			case *pb.IpvsService_TcpAttributes:
				vip = attr.TcpAttributes.GetAddress()
			case *pb.IpvsService_UdpAttributes:
				vip = attr.UdpAttributes.GetAddress()
			case *pb.IpvsService_FwmarkAttributes:
				// skipping fwmarks since they are vipless.
				continue
			}

			if _, ok := addrMap[vip.String()]; !ok {
				addrMap[vip.String()] = &pb.LinkAddress{
					Address:  vip,
					LinkName: common.LoopbackLinkName,
				}
			}
		}
	}

	addrs := make([]*pb.LinkAddress, 0, len(addrMap))
	for _, val := range addrMap {
		addrs = append(addrs, val)
	}
	return addrs, nil
}

// generate list of routes allowed to be announced based on filtering allowed
// through prohibited routes.
func (s *ControlPlaneServicer) generateRoutes(
	allowedRoutes,
	prohibitedRoutes []*pb.BgpRouteAttributes) ([]*pb.DynamicRoute, error) {

	// result list.
	var result []*pb.DynamicRoute
	// map of prefixes to skip dups.
	resultMap := make(map[string]bool)

	// preparing list of routes to announce after filtering them through
	// prohibited list.
	for _, allowedRoute := range allowedRoutes {
		_, allowedNet, err := net.ParseCIDR(fmt.Sprintf(
			"%s/%d",
			common.KglbAddrToNetIp(allowedRoute.GetPrefix()),
			allowedRoute.GetPrefixlen()))
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"fails to parse route: %+v",
				allowedRoute)
		}

		found := false
		for _, prohibitedRoute := range prohibitedRoutes {
			_, prohibitedNet, err := net.ParseCIDR(fmt.Sprintf(
				"%s/%d",
				common.KglbAddrToNetIp(prohibitedRoute.GetPrefix()),
				prohibitedRoute.GetPrefixlen()))
			if err != nil {
				return nil, errors.Wrapf(
					err,
					"fails to parse route: %+v",
					prohibitedRoute)
			}

			if prohibitedNet.Contains(allowedNet.IP) {
				dlog.Info("prefix is not allowed to be announced yet: ", allowedNet.String())
				found = true
				break
			}
		}
		if !found {
			// excluding dups.
			if _, ok := resultMap[allowedNet.String()]; !ok {
				result = append(
					result,
					&pb.DynamicRoute{
						Attributes: &pb.DynamicRoute_BgpAttributes{
							BgpAttributes: allowedRoute,
						},
					})
				resultMap[allowedNet.String()] = true
			}
		}
	}

	return result, nil
}

func perUpstreamHash(s string, u uint32) uint64 {
	fnvHash := fnv.New64()
	fnvHash.Write([]byte(fmt.Sprintf("%s_%d", s, u)))
	return fnvHash.Sum64()
}

func (s *ControlPlaneServicer) setStatesHashes(hashes map[string]uint64) {
	for k, v := range hashes {
		if g, exists := s.perSetupStateHashes[k]; exists {
			g.Set(float64(v))
		} else {
			g, err := stateHashGauge.V(v2stats.KV{
				"setup":  k,
				"entity": "global",
			})
			if err != nil {
				exclog.Report(errors.Wrapf(err,
					"unable to create gauge for %s setup", k),
					exclog.Critical, "")
				return
			}
			g.Set(float64(v))
			s.perSetupStateHashes[k] = &g
		}
	}
}

func (s *ControlPlaneServicer) GenerateDataPlaneState() (
	*pb.DataPlaneState, error) {

	// list of routes are not allowed to announce since balancers may not be
	// ready to handle traffic yet.
	var prohibitedRoutes []*pb.BgpRouteAttributes
	// list of allowed routes.
	var allowedRoutes []*pb.BgpRouteAttributes

	// number of fully uninitialized balancers.
	statelessBalancerCnt := 0

	result := &pb.DataPlaneState{}

	existingFwmarks := make(map[uint32]bool)
	// stateHashPerSetup is map, which contains setup's (key) specific hash (value). kglbs in same cluster+setup pair
	// must have the same hash value to be consistent amongs each other
	stateHashPerSetup := make(map[string]uint64)
	for _, balancer := range s.balancers {
		balancerState := balancer.GetState()
		balancerConfig := balancer.GetConfig()

		for _, state := range balancerState.States {
			fwm := state.GetLbService().GetIpvsService().GetFwmarkAttributes().GetFwmark()
			if fwm > 0 {
				if _, exists := existingFwmarks[fwm]; exists {
					continue
				} else {
					existingFwmarks[fwm] = true
				}
			} else {
				for _, upstream := range state.GetUpstreams() {
					stateHashPerSetup[balancerConfig.SetupName] += perUpstreamHash(upstream.GetHostname()+state.GetName(), upstream.GetWeight())
				}
			}
			result.Balancers = append(result.Balancers, state)
		}
		confRatio := balancerConfig.GetDynamicRouting().GetAnnounceLimitRatio()

		// Counting balancers without states, it generates states only after
		// receiving update from health manager which does it after discovery and
		// health checking.
		if len(balancerState.States) == 0 {
			statelessBalancerCnt += 1
		}

		// v2 route_announcement gauge tags
		tags := v2stats.KV{
			"setup":   balancerConfig.SetupName,
			"service": balancerConfig.Name,
		}

		// do not announce route in initial state since alive ratio may be
		// still zero during this period.
		if balancerConfig.GetDynamicRouting().GetBgpAttributes() != nil &&
			!balancerState.InitialState &&
			balancerState.AliveRatio > 0 &&
			balancerState.AliveRatio >= confRatio {

			allowedRoutes = append(
				allowedRoutes,
				balancerConfig.GetDynamicRouting().GetBgpAttributes())
			tags["state"] = "on"
		} else {
			dlog.Infof(
				"skipping announcing route for balancer: %s, %+v, %+v",
				balancerConfig.GetName(),
				balancerConfig.GetDynamicRouting().GetBgpAttributes(),
				balancerState)
			prohibitedRoutes = append(
				prohibitedRoutes,
				balancerConfig.GetDynamicRouting().GetBgpAttributes())
			tags["state"] = "off"
		}

		err := s.statRouteAnnouncement.PrepareToSet(1, tags)
		if err != nil {
			exclog.Report(errors.Wrapf(err,
				"unable to instantiate routeAnnouncement metric (tags %v)", tags),
				exclog.Critical, "")
		}
	}
	s.setStatesHashes(stateHashPerSetup)
	s.statRouteAnnouncement.SetAndReset()

	// generating routes to announce.
	var err error
	result.DynamicRoutes, err = s.generateRoutes(allowedRoutes, prohibitedRoutes)
	if err != nil {
		return nil, err
	}

	// generate link addresses.
	linkAddresses, err := s.generateLinkAddrs()
	if err != nil {
		return nil, err
	}
	result.LinkAddresses = linkAddresses

	// validating generate state.
	if err = common.ValidateDataPlaneState(result); err != nil {
		return nil, err
	}

	if statelessBalancerCnt == 0 {
		s.initialState = false
	}

	return result, nil
}

// Applying data plane state.
func (s *ControlPlaneServicer) applyDataPlaneState(state *pb.DataPlaneState) error {
	if err := s.modules.DataPlaneClient.Set(state); err != nil {
		return err
	} else {
		if !s.initialState && s.modules.AfterInitHandler != nil {
			s.modules.AfterInitHandler()
			// loosing reference to callback to avoid it call again.
			s.modules.AfterInitHandler = nil
		}
	}

	return nil
}

// Sends state to DP based on ticker or notification from balancer.
func (s *ControlPlaneServicer) startBackgroundPusher(duration time.Duration) {
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	// Reporting availability metric based on result of applying dp state (average of the
	// stat gives us availability percentage since zero is reported in failure and one
	// in success). Failure of applying state catches different set of issues including
	// network configuration/bgp session/ipvs and non-working data plane.
	for {
		select {
		case <-s.ctx.Done():
			dlog.Info("closing control plane and event loop...")
			return
		case config := <-s.modules.ConfigLoader.Updates():
			// config updates.
			if err := s.updateConfig(config.(*pb.ControlPlaneConfig)); err != nil {
				exclog.Report(err, exclog.Critical, "")
			}
		case <-ticker.C:
			// getting ref to the state since it might be updated.
			s.mu.Lock()
			state := s.state
			s.mu.Unlock()

			if state != nil {
				// periodically re-applying state just in case data plane lost
				// it, for example after restarting.
				dlog.Info("sending state caused by ticker: ", state)
				if err := s.applyDataPlaneState(state); err != nil {
					s.updateAvailabilityGauge(0)
					exclog.Report(err, exclog.Critical, "")
				} else {
					s.updateAvailabilityGauge(1)
				}
			} else {
				s.updateAvailabilityGauge(0)
			}
		case <-s.balancersUpdatesChan:
			state, err := s.GenerateDataPlaneState()
			if err != nil {
				exclog.Report(err, exclog.Critical, "")
				continue
			}
			s.mu.Lock()
			s.state = state
			s.mu.Unlock()

			// finally apply data plane state.
			dlog.Info("sending state because of balancers updates: ", state)
			if err := s.applyDataPlaneState(state); err != nil {
				s.updateAvailabilityGauge(0)
				exclog.Report(err, exclog.Critical, "")
			} else {
				s.updateAvailabilityGauge(1)
			}
		}
	}
}

func (s *ControlPlaneServicer) updateAvailabilityGauge(value int) {
	tags := v2stats.KV{"initialized": "true"}
	if s.initialState {
		tags["initialized"] = "false"
	}
	err := s.statAvailability.PrepareToSet(float64(value), tags)
	if err != nil {
		exclog.Report(errors.Wrap(err,
			"unable to instantiate availability metric"), exclog.Critical, "")
		return
	}

	s.statAvailability.SetAndReset()
}
