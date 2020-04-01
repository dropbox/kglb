package data_plane

import (
	. "gopkg.in/check.v1"

	kglb_pb "dropbox/proto/kglb"
	. "godropbox/gocheck2"
)

type BalancerManagerSuite struct {
	// Mock IPVS module.
	mockIpvs *MockIpvsModule
	// BalancerManager.
	manager *BalancerManager
}

var _ = Suite(&BalancerManagerSuite{})

func (m *BalancerManagerSuite) SetUpTest(c *C) {
	m.mockIpvs = &MockIpvsModule{}

	resolver, err := NewResolver()
	c.Assert(err, IsNil)

	params := BalancerManagerParams{
		Ipvs:     m.mockIpvs,
		Resolver: resolver,
	}

	m.manager, err = NewBalancerManager(params)
	c.Assert(err, IsNil)
}

// Test AddBalancer.
func (m *BalancerManagerSuite) TestAddBalancer(c *C) {
	// 1. empty list of services.
	m.mockIpvs.ListServicesFunc = func() ([]*kglb_pb.IpvsService, []*kglb_pb.Stats, error) {
		return []*kglb_pb.IpvsService{}, []*kglb_pb.Stats{}, nil
	}
	m.mockIpvs.AddServiceFunc = func(service *kglb_pb.IpvsService) error {
		c.Assert(
			service,
			DeepEqualsPretty,
			&kglb_pb.IpvsService{
				Attributes: &kglb_pb.IpvsService_TcpAttributes{
					TcpAttributes: &kglb_pb.IpvsTcpAttributes{
						Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
						Port:    443,
					}},
				Scheduler: kglb_pb.IpvsService_RR,
			})
		return nil
	}

	m.mockIpvs.AddRealServerFunc = func(service *kglb_pb.IpvsService,
		dst *kglb_pb.UpstreamState) error {

		// expecting 10.0.0.4 to be added only.
		c.Assert(dst, DeepEqualsPretty, &kglb_pb.UpstreamState{
			Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
			Port:          443,
			Hostname:      "hostname1",
			Weight:        50,
			ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
		})

		return nil
	}

	err := m.manager.AddBalancer(&kglb_pb.BalancerState{
		Name: "TestName1",
		LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
			IpvsService: &kglb_pb.IpvsService{
				Attributes: &kglb_pb.IpvsService_TcpAttributes{
					TcpAttributes: &kglb_pb.IpvsTcpAttributes{
						Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
						Port:    443,
					}},
				Scheduler: kglb_pb.IpvsService_RR,
			}}},
		Upstreams: []*kglb_pb.UpstreamState{
			{
				Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
				Port:          443,
				Hostname:      "hostname1",
				Weight:        50,
				ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
			},
		},
	})
	c.Assert(err, IsNil)
}
