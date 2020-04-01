package data_plane

import (
	"sync"
	"time"

	"dropbox/dlog"
	"dropbox/exclog"
	kglb_pb "dropbox/proto/kglb"
	"dropbox/vortex2/v2stats"
	"godropbox/errors"
)

type DynamicRoutingManagerParams struct {
	Bgp BgpModule
}

type DynamicRoutingManager struct {
	params *DynamicRoutingManagerParams
	mu     sync.Mutex
	// map of hold timeouts per "prefix" key.
	holdTimeouts map[string]time.Duration

	bgpSessionStat *v2stats.GaugeGroup
	bgpRouteStat   *v2stats.GaugeGroup
}

func NewDynamicRoutingManager(
	params DynamicRoutingManagerParams) (*DynamicRoutingManager, error) {
	m := &DynamicRoutingManager{
		params:         &params,
		holdTimeouts:   make(map[string]time.Duration),
		bgpSessionStat: v2stats.NewGaugeGroup(bgpSessionStateGauge),
		bgpRouteStat:   v2stats.NewGaugeGroup(bgpRouteGauge),
	}

	m.startStatsCollector()

	return m, nil
}

func (m *DynamicRoutingManager) startStatsCollector() {
	ticker := time.NewTicker(time.Second * 10)
	go func() {
		for range ticker.C {
			m.collectBgpStats()
		}
	}()
}

func (m *DynamicRoutingManager) collectBgpStats() {
	bgpSessionEstablished, err := m.IsSessionEstablished()
	if err != nil {
		exclog.Report(
			errors.Wrap(err, "failed to call IsSessionEstablished: "), exclog.Operational, "")
		return
	}

	bgpState := "not_established"
	if bgpSessionEstablished {
		bgpState = "established"
	}

	err = m.bgpSessionStat.PrepareToSet(1, v2stats.KV{"state": bgpState})
	if err != nil {
		exclog.Report(
			errors.Wrap(err, "unable to PrepareToSet BgpSessionGauge"), exclog.Critical, "")
	}
	m.bgpSessionStat.SetAndReset()

	//TODO(oleg): emits stats for IPv4 and IPv6 with different tags
	routesAdvertised, err := m.ListRoutes()
	if err != nil {
		exclog.Report(
			errors.Wrap(err, "failed to call ListRoutes: "), exclog.Noncritical, "")
		return
	}

	for _, dynamicRoute := range routesAdvertised {
		bgpRoute, err := m.getBgpRouting(dynamicRoute)
		if err != nil {
			exclog.Report(
				errors.Wrap(err, "failed to get BGP route: "), exclog.Noncritical, "")
			continue
		}
		err = m.bgpRouteStat.PrepareToSet(1, v2stats.KV{"route": getBgpRouteName(bgpRoute)})
		if err != nil {
			exclog.Report(
				errors.Wrap(err, "unable to PrepareToSet BgpRouteGauge"), exclog.Critical, "")
		}
	}
	// Emit all currently existing routes / reset removed routes
	m.bgpRouteStat.SetAndReset()
}

func (m *DynamicRoutingManager) getBgpRouting(
	routing *kglb_pb.DynamicRoute) (*kglb_pb.BgpRouteAttributes, error) {

	switch routing.Attributes.(type) {
	case *kglb_pb.DynamicRoute_BgpAttributes:
		return routing.GetBgpAttributes(), nil
	default:
		return nil, errors.Newf("Unknown routing attribute: %+v", routing)
	}
}

func (m *DynamicRoutingManager) ListRoutes() ([]*kglb_pb.DynamicRoute, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bgpRoutes, err := m.params.Bgp.ListPaths()
	if err != nil {
		exclog.Report(errors.Wrapf(err, "fails to list routes:"), exclog.Operational, "")
		return nil, err
	}

	results := make([]*kglb_pb.DynamicRoute, len(bgpRoutes))
	for i, route := range bgpRoutes {
		// adding hold timeout if it's in the cache.
		if val, ok := m.holdTimeouts[route.GetPrefix().String()]; ok {
			route.HoldTimeMs = uint32(val / time.Millisecond)
		}
		results[i] = &kglb_pb.DynamicRoute{
			Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
				BgpAttributes: route,
			},
		}
	}
	return results, nil
}

// Advertise set of routes.
func (m *DynamicRoutingManager) AdvertiseRoutes(
	routes []*kglb_pb.DynamicRoute) error {

	// one call at a time and protect internal map of timeouts.
	m.mu.Lock()
	defer m.mu.Unlock()

	size := len(routes)

	for i, route := range routes {
		dlog.Infof("[%d/%d] Advertising route: %+v", i+1, size, route)
		if err := m.advertiseRouteLocked(route); err != nil {
			exclog.Report(errors.Wrapf(err, "fails to advertise route:"), exclog.Operational, "")
			return err
		}
	}
	return nil
}

// Advertise single route (non-thread safe).
func (m *DynamicRoutingManager) advertiseRouteLocked(route *kglb_pb.DynamicRoute) (err error) {
	// supporting bgp attributes for now only.
	bgpRoute, err := m.getBgpRouting(route)
	if err != nil {
		return errors.Wrap(err, "failed to extract bgp attributes: ")
	}

	// init speaker with ASN
	if err = m.params.Bgp.Init(bgpRoute.GetLocalAsn()); err != nil {
		return errors.Wrap(err, "failed to init bgp session: ")
	}
	// advertise BGP path
	if err = m.params.Bgp.Advertise(bgpRoute); err != nil {
		return errors.Wrap(err, "failed to advertise bgp path: ")
	}

	// update timeouts map.
	m.holdTimeouts[bgpRoute.GetPrefix().String()] =
		time.Duration(bgpRoute.GetHoldTimeMs()) * time.Millisecond

	return nil
}

// Withdraw set of routes. At the end of the call it waits max hold timeout
// across all withdrawn routes.
func (m *DynamicRoutingManager) WithdrawRoutes(
	routes []*kglb_pb.DynamicRoute) error {

	// one call at a time and protect internal map of timeouts.
	m.mu.Lock()
	defer m.mu.Unlock()

	size := len(routes)
	// track max hold timeout across all withdrawing routes to apply max value.
	maxTimeout := time.Duration(0)

	for i, route := range routes {
		dlog.Infof("[%d/%d] Withdrawing route: %+v", i+1, size, route)
		if holdTimeout, err := m.withdrawRouteLocked(route); err != nil {
			exclog.Report(errors.Wrapf(err, "fails to withdraw route: %v", route), exclog.Operational, "")
			return err
		} else {
			// updating timeout to wait after withdrawing all routes.
			if holdTimeout > maxTimeout {
				maxTimeout = holdTimeout
			}
		}
	}

	if maxTimeout > 0 {
		dlog.Infof("waiting max hold timeout: %v", maxTimeout)
		time.Sleep(maxTimeout)
	}

	return nil
}

// (non-thread safe) withdraw single route and returns hold timeouts for
// specifically for removed route or error.
func (m *DynamicRoutingManager) withdrawRouteLocked(
	route *kglb_pb.DynamicRoute) (time.Duration, error) {

	bgpRoute, err := m.getBgpRouting(route)
	if err != nil {
		dlog.Errorf("fails to extract bgp attributes: %v", err)
		return 0, err
	}

	if err := m.params.Bgp.Withdraw(bgpRoute); err != nil {
		return 0, errors.Wrapf(err, "failed to withdraw dynamic route: %+v", route)
	}

	// extracting holding timeout.
	holdTimeout := time.Duration(bgpRoute.GetHoldTimeMs()) * time.Millisecond

	// removing entry from the map since route is already deleted.
	key := bgpRoute.GetPrefix().String()
	if _, ok := m.holdTimeouts[key]; ok {
		delete(m.holdTimeouts, key)
	}

	return holdTimeout, nil
}

func (m *DynamicRoutingManager) IsSessionEstablished() (bool, error) {
	return m.params.Bgp.IsSessionEstablished()
}
