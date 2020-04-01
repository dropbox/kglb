package data_plane

import (
	"dropbox/dlog"
	"dropbox/kglb/common"
	kglb_pb "dropbox/proto/kglb"
	"godropbox/errors"
)

type BalancerManagerParams struct {
	Ipvs IpvsModule
	// DNS related module.
	Resolver ResolverModule
}

type BalancerManager struct {
	params *BalancerManagerParams
}

func NewBalancerManager(
	params BalancerManagerParams) (*BalancerManager, error) {

	return &BalancerManager{
		params: &params,
	}, nil
}

// Get existent balancers.
func (m *BalancerManager) GetBalancers() ([]*kglb_pb.BalancerState, error) {
	// return value.
	balancers := []*kglb_pb.BalancerState{}

	ipvsServices, _, err := m.params.Ipvs.ListServices()
	if err != nil {
		return nil, errors.Wrap(
			err,
			"GetBalancer() fails because of Ipvs.ListServices(): ")
	}

	for _, ipvsService := range ipvsServices {
		// find service name.
		serviceName := m.params.Resolver.ServiceLookup(ipvsService)

		// query upstreams.
		configuredReals, _, err := m.params.Ipvs.GetRealServers(ipvsService)
		if err != nil {
			return nil, errors.Wrap(
				err,
				"GetBalancer() fails because of GetRealServers(): ")
		}

		balancers = append(balancers, &kglb_pb.BalancerState{
			Name: serviceName,
			LbService: &kglb_pb.LoadBalancerService{
				Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: ipvsService,
				},
			},
			Upstreams: configuredReals,
		})
	}

	return balancers, nil
}

// Add set of new balancers.
func (m *BalancerManager) AddBalancers(balancers []*kglb_pb.BalancerState) error {
	for _, balancer := range balancers {
		if err := m.AddBalancer(balancer); err != nil {
			return errors.Wrapf(err, "failed to add balancer: %+v", balancer)
		} else {
			dlog.Infof("Balancer has been added: %s", balancer.GetName())
		}
	}
	return nil
}

// Add new balancer.
func (m *BalancerManager) AddBalancer(balancer *kglb_pb.BalancerState) error {
	// 1. extracting ipvs service.
	ipvsService, err := common.GetIpvsServiceFromBalancer(balancer)
	if err != nil {
		return errors.Wrapf(err, "fails to extract ipvs service from balancer: ")
	}

	// 2. adding ipvs service.
	if err := m.params.Ipvs.AddService(ipvsService); err != nil {
		return errors.Wrapf(err, "failed to create IPVS service: ")
	}

	// 3. adding reals.
	reals := balancer.GetUpstreams()
	if err := m.params.Ipvs.AddRealServers(ipvsService, reals); err != nil {
		return errors.Wrapf(err, "failed to add upstreams: ")
	}

	return nil
}

// Delete set of existent balancers.
func (m *BalancerManager) DeleteBalancers(balancers []*kglb_pb.BalancerState) error {
	for _, balancer := range balancers {
		if err := m.DeleteBalancer(balancer); err != nil {
			return errors.Wrapf(err, "failed to delete balancer: %+v", balancer)
		} else {
			dlog.Infof("Balancer has been deleted : %s", balancer.GetName())
		}
	}
	return nil
}

func (m *BalancerManager) DeleteBalancer(balancer *kglb_pb.BalancerState) error {
	// 1. extracting ipvs service.
	ipvsService, err := common.GetIpvsServiceFromBalancer(balancer)
	if err != nil {
		return errors.Wrapf(err, "fails to extract ipvs service from balancer: ")
	}

	// 2. deleting ipvs service.
	if err := m.params.Ipvs.DeleteService(ipvsService); err != nil {
		return errors.Wrapf(err, "failed to delete IPVS service: ")
	}
	return nil
}

// Add new upstreams for specific LoadBalancerService.
func (m *BalancerManager) AddUpstreams(
	lbService *kglb_pb.LoadBalancerService,
	upstreams []*kglb_pb.UpstreamState) error {

	// 1. extracting ipvs service.
	ipvsService, err := common.GetIpvsServiceFromLbService(lbService)
	if err != nil {
		return errors.Wrapf(err, "fails to extract ipvs service from LoadBalancerService: ")
	}

	// 2. adding upstreams.
	if err := m.params.Ipvs.AddRealServers(ipvsService, upstreams); err != nil {
		return errors.Wrapf(err, "failed to add upstreams: ")
	}
	return nil
}

// Delete upstreams from specific LoadBalancerService.
func (m *BalancerManager) DeleteUpstreams(
	lbService *kglb_pb.LoadBalancerService,
	upstreams []*kglb_pb.UpstreamState) error {

	// 1. extracting ipvs service.
	ipvsService, err := common.GetIpvsServiceFromLbService(lbService)
	if err != nil {
		return errors.Wrapf(err, "fails to extract ipvs service from LoadBalancerService: ")
	}

	// 2. deleting upstreams.
	if err := m.params.Ipvs.DeleteRealServers(ipvsService, upstreams); err != nil {
		return errors.Wrapf(err, "failed to delete upstreams: ")
	}
	return nil
}

// Update upstreams for specific LoadBalancerService.
func (m *BalancerManager) UpdateUpstreams(
	lbService *kglb_pb.LoadBalancerService,
	upstreams []*kglb_pb.UpstreamState) error {

	// 1. extracting ipvs service.
	ipvsService, err := common.GetIpvsServiceFromLbService(lbService)
	if err != nil {
		return errors.Wrapf(err, "fails to extract ipvs service from LoadBalancerService: ")
	}

	// 2. updating upstreams.
	if err := m.params.Ipvs.UpdateRealServers(ipvsService, upstreams); err != nil {
		return errors.Wrapf(err, "failed to update upstreams: ")
	}

	return nil
}
