package data_plane

import (
	"fmt"
	"net"
	"time"

	. "gopkg.in/check.v1"

	kglb_pb "dropbox/proto/kglb"
	. "godropbox/gocheck2"
)

type ManagerSuite struct {
}

var _ = Suite(&ManagerSuite{})

// Perform SetState() and GetState() on top of mock modules with states.
func (m *ManagerSuite) TestSimpleSetGetState(c *C) {
	modules, err := GetMockModules(nil)
	c.Assert(err, IsNil)
	mng, err := NewManager(*modules)
	c.Assert(err, IsNil)
	c.Assert(time.Since(mng.lastSuccessfulStateChange), LessThan, time.Hour)

	// 1. construct BalancerState
	testState := &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
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
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"},
						},
						Prefixlen:  32,
						HoldTimeMs: 51,
					},
				},
			},
		},
		LinkAddresses: []*kglb_pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			},
		},
	}

	// 2. Perform SetState().
	oldTs := mng.lastSuccessfulStateChange
	err = mng.SetState(testState)
	c.Assert(err, IsNil)
	c.Assert(mng.lastSuccessfulStateChange, Not(Equals), oldTs)
	c.Assert(time.Since(mng.lastSuccessfulStateChange), LessThan, time.Hour)

	// 3. Perform GetState().
	newState, err := mng.GetState()
	c.Assert(err, IsNil)
	// double checking we got what we just applied (except balancer names since
	// current resolver doesn't cache values).
	c.Assert(newState, DeepEqualsPretty, &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
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
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"},
						},
						Prefixlen:  32,
						HoldTimeMs: 51,
					},
				},
			},
		},
		LinkAddresses: []*kglb_pb.LinkAddress{
			{
				LinkName: "lo",
				Address: &kglb_pb.IP{
					Address: &kglb_pb.IP_Ipv4{
						Ipv4: "172.0.0.1",
					},
				},
			},
		},
	})

	// checking addresses.
	addrExist, err := modules.AddressTable.IsExists(
		net.ParseIP("172.0.0.1"),
		"lo")
	c.Assert(err, IsNil)
	c.Assert(addrExist, IsTrue)

	// 5. Perform GetState().
	newState, err = mng.GetState()
	c.Assert(err, IsNil)
	c.Assert(newState, DeepEqualsPretty, &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
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
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"},
						},
						Prefixlen:  32,
						HoldTimeMs: 51,
					},
				},
			},
		},
		LinkAddresses: []*kglb_pb.LinkAddress{
			{
				LinkName: "lo",
				Address: &kglb_pb.IP{
					Address: &kglb_pb.IP_Ipv4{
						Ipv4: "172.0.0.1",
					},
				},
			},
		},
	})
}

func (m *ManagerSuite) TestUpdateUpstreamWeight(c *C) {
	modules, err := GetMockModules(nil)
	c.Assert(err, IsNil)
	mng, err := NewManager(*modules)
	c.Assert(err, IsNil)

	// 1. construct initialState.
	testState := &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
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
			},
		},
	}

	// 2. Perform SetState().
	err = mng.SetState(testState)
	c.Assert(err, IsNil)

	// 3. Update weight of one of upstream.
	testState = &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
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
						Weight:        90,
						ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
					},
				},
			},
		},
	}
	err = mng.SetState(testState)
	c.Assert(err, IsNil)

	// 4. Perform GetState().
	newState, err := mng.GetState()
	c.Assert(err, IsNil)
	// double checking we got what we just applied (except balancer names since
	// current resolver doesn't cache values).
	c.Assert(newState, DeepEqualsPretty, &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
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
						Weight:        90,
						ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
					},
				},
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{},
	})
}

func (m *ManagerSuite) TestDeleteBalancer(c *C) {
	modules, err := GetMockModules(nil)
	c.Assert(err, IsNil)
	mng, err := NewManager(*modules)
	c.Assert(err, IsNil)

	// 1. construct initialState.
	testState := &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
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
			},
			{
				Name: "TestName2",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					}}},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
						Port:          443,
						Hostname:      "hostname2",
						Weight:        50,
						ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
					},
				},
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"},
						},
						Prefixlen: 32,
					},
				},
			},
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  30,
						PeerAsn:   40,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.5"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
		LinkAddresses: []*kglb_pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			},
			{
				LinkName: "lo",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
			},
		},
	}

	// 2. Perform SetState().
	err = mng.SetState(testState)
	c.Assert(err, IsNil)
	// validate before moving forward.
	newState, err := mng.GetState()
	c.Assert(err, IsNil)
	c.Assert(len(newState.GetBalancers()), Equals, 2)
	// validate addresses.
	allIps, err := modules.AddressTable.List("lo")
	c.Assert(err, IsNil)
	c.Assert(len(allIps), Equals, 3)
	addrExist, err := modules.AddressTable.IsExists(
		net.ParseIP("172.0.0.1"),
		"lo")
	c.Assert(err, IsNil)
	c.Assert(addrExist, IsTrue)
	addrExist, err = modules.AddressTable.IsExists(
		net.ParseIP("172.0.0.2"),
		"lo")
	c.Assert(err, IsNil)
	c.Assert(addrExist, IsTrue)

	// 3. Removing one balancer.
	testState = &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
				Name: "TestName2",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					}}},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
						Port:          443,
						Hostname:      "hostname2",
						Weight:        50,
						ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
					},
				},
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  30,
						PeerAsn:   40,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.5"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
		LinkAddresses: []*kglb_pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
			},
		},
	}
	err = mng.SetState(testState)
	c.Assert(err, IsNil)

	// 4. Validate state.
	newState, err = mng.GetState()
	c.Assert(err, IsNil)
	// validate before moving forward.
	c.Assert(len(newState.GetBalancers()), Equals, 1)

	// validate addresses.
	allIps, err = modules.AddressTable.List("lo")
	c.Assert(err, IsNil)
	c.Assert(len(allIps), Equals, 2)
	addrExist, err = modules.AddressTable.IsExists(
		net.ParseIP("172.0.0.1"),
		"lo")
	c.Assert(err, IsNil)
	c.Assert(addrExist, IsFalse)
	addrExist, err = modules.AddressTable.IsExists(
		net.ParseIP("172.0.0.2"),
		"lo")
	c.Assert(err, IsNil)
	c.Assert(addrExist, IsTrue)

	// double checking we got what we just applied (except balancer names since
	// current resolver doesn't cache values).
	c.Assert(newState, DeepEqualsPretty, &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
				Name: "TestName2",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					}}},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
						Port:          443,
						Hostname:      "hostname2",
						Weight:        50,
						ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
					},
				},
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  30,
						PeerAsn:   40,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.5"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
		LinkAddresses: []*kglb_pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
			},
		},
	})
}

func (m *ManagerSuite) TestSetFwMarkBalancer(c *C) {
	modules, err := GetMockModules(nil)
	c.Assert(err, IsNil)
	mng, err := NewManager(*modules)
	c.Assert(err, IsNil)

	// 1. construct BalancerState
	testState := &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
				Name: "TestName1",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_FwmarkAttributes{
							FwmarkAttributes: &kglb_pb.IpvsFwmarkAttributes{
								Fwmark: 10,
							},
						},
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
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
	}

	// 2. Perform SetState().
	err = mng.SetState(testState)
	c.Assert(err, IsNil)

	// 3. Perform GetState().
	newState, err := mng.GetState()
	c.Assert(err, IsNil)
	// double checking we got what we just applied (except balancer names since
	// current resolver doesn't cache values).
	c.Assert(newState, DeepEqualsPretty, &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
				Name: "TestName1",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_FwmarkAttributes{
							FwmarkAttributes: &kglb_pb.IpvsFwmarkAttributes{
								AddressFamily: kglb_pb.AddressFamily_AF_INET,
								Fwmark:        10,
							},
						},
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
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
	})
}

func (m *ManagerSuite) TestShutdown(c *C) {
	modules, err := GetMockModules(nil)
	c.Assert(err, IsNil)
	mng, err := NewManager(*modules)
	c.Assert(err, IsNil)

	// 1. construct initialState.
	testState := &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
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
			},
			{
				Name: "TestName2",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					}}},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
						Port:          443,
						Hostname:      "hostname2",
						Weight:        50,
						ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
					},
				},
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  65101,
						PeerAsn:   20,
						Community: "65101:30000",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"},
						},
						Prefixlen: 32,
					},
				},
			},
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  65102,
						PeerAsn:   40,
						Community: "65102:30000",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.5"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
		LinkAddresses: []*kglb_pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			},
			{
				LinkName: "lo",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
			},
		},
	}

	// 2. Perform SetState().
	err = mng.SetState(testState)
	c.Assert(err, IsNil)

	// 3. Validate that state is applied.
	newState, err := mng.GetState()
	c.Assert(err, IsNil)
	// validate before moving forward.
	c.Assert(len(newState.GetBalancers()), Equals, 2)
	paths, err := modules.Bgp.ListPaths()
	c.Assert(err, IsNil)
	c.Assert(len(paths), Equals, 2)
	addrs, err := modules.AddressTable.List("lo")
	c.Assert(err, IsNil)
	c.Assert(len(addrs), Equals, 3)

	// 4. Perform Shutdown().
	err = mng.Shutdown()
	c.Assert(err, IsNil)
	c.Assert(mng.shutdownOnce, IsTrue)
	// validate modules.
	newState, err = mng.GetState()
	c.Assert(err, IsNil)
	c.Assert(len(newState.GetBalancers()), Equals, 0)
	paths, err = modules.Bgp.ListPaths()
	c.Assert(err, IsNil)
	c.Assert(len(paths), Equals, 0)
	addrs, err = modules.AddressTable.List("lo")
	c.Assert(err, IsNil)
	c.Assert(len(addrs), Equals, 1)

	// 5. SetState should fails.
	oldTs := mng.lastSuccessfulStateChange
	err = mng.SetState(testState)
	c.Assert(err, NotNil)
	c.Assert(mng.lastSuccessfulStateChange, Equals, oldTs)
}

// validate data plane behavior in case of multiple BalancerStates with the same
// name, but different VIPs.
func (m *ManagerSuite) TestDupBalancerNames(c *C) {
	modules, err := GetMockModules(nil)
	c.Assert(err, IsNil)
	mng, err := NewManager(*modules)
	c.Assert(err, IsNil)

	// 1. construct BalancerState
	testState := &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
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
			},
			{
				Name: "TestName1",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
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
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
		LinkAddresses: []*kglb_pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			},
			{
				LinkName: "lo",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
			},
		},
	}

	// 2. Perform SetState().
	err = mng.SetState(testState)
	c.Assert(err, IsNil)

	// 3. Perform GetState().
	newState, err := mng.GetState()
	c.Assert(err, IsNil)
	c.Assert(len(newState.Balancers), Equals, 2)
	// 'default' because of resolver.
	c.Assert(newState.Balancers[0].GetName(), Equals, "TestName1")

	// checking addresses.
	addrExist, err := modules.AddressTable.IsExists(
		net.ParseIP("172.0.0.1"),
		"lo")
	c.Assert(err, IsNil)
	c.Assert(addrExist, IsTrue)
	addrExist, err = modules.AddressTable.IsExists(
		net.ParseIP("172.0.0.2"),
		"lo")
	c.Assert(err, IsNil)
	c.Assert(addrExist, IsTrue)
}

// Validate that withdraw timeout is really applied.
func (m *ManagerSuite) TestSetConfigurationAddingService(c *C) {
	ipvsModule := NewMockIpvsModuleWithState().(*MockIpvsModuleWithState)
	bgpModule := NewMockBgpModuleWithState().(*MockBgpModule)

	modules, err := GetMockModules(&ManagerModules{
		Ipvs: ipvsModule,
		Bgp:  bgpModule,
	})

	c.Assert(err, IsNil)
	mng, err := NewManager(*modules)
	c.Assert(err, IsNil)

	// 1. construct BalancerState
	state := &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
				Name: "TestName1",
				LbService: &kglb_pb.LoadBalancerService{
					Service: &kglb_pb.LoadBalancerService_IpvsService{
						IpvsService: &kglb_pb.IpvsService{
							Attributes: &kglb_pb.IpvsService_TcpAttributes{
								TcpAttributes: &kglb_pb.IpvsTcpAttributes{
									Address: &kglb_pb.IP{
										Address: &kglb_pb.IP_Ipv4{
											Ipv4: "172.0.0.1",
										},
									},
									Port: 443,
								},
							},
							Scheduler: kglb_pb.IpvsService_RR,
						},
					},
				},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
						Port:          443,
						Hostname:      "hostname1",
						Weight:        50,
						ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
					},
				},
			},
		},
		DynamicRoutes: []*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"},
						},
						Prefixlen:  32,
						HoldTimeMs: 50,
					},
				},
			},
		},
	}

	var serviceTime time.Time
	ipvsModule.MockIpvsModule.DeleteServiceFunc = func(service *kglb_pb.IpvsService) error {
		serviceTime = time.Now()
		return nil
	}

	var withdrawTime time.Time
	bgpModule.WithdrawFunc = func(bgpConfig *kglb_pb.BgpRouteAttributes) error {
		withdrawTime = time.Now()
		return nil
	}

	// 3. Apply state.
	err = mng.SetState(state)
	c.Assert(err, NoErr)

	// check value in constructed state.
	stateCheck, err := mng.GetState()
	c.Assert(err, IsNil)
	c.Assert(stateCheck, DeepEqualsPretty, state)

	// check that timeout is applied.
	state.DynamicRoutes = []*kglb_pb.DynamicRoute{
		{
			Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
				BgpAttributes: &kglb_pb.BgpRouteAttributes{
					LocalAsn:  10,
					PeerAsn:   20,
					Community: "my_community",
					Prefix: &kglb_pb.IP{
						Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.5"},
					},
					Prefixlen:  32,
					HoldTimeMs: 50,
				},
			},
		},
	}
	state.Balancers = []*kglb_pb.BalancerState{
		{
			Name: "TestName2",
			LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
				IpvsService: &kglb_pb.IpvsService{
					Attributes: &kglb_pb.IpvsService_TcpAttributes{
						TcpAttributes: &kglb_pb.IpvsTcpAttributes{
							Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
							Port:    443,
						}},
					Scheduler: kglb_pb.IpvsService_RR,
				}}},
			Upstreams: []*kglb_pb.UpstreamState{
				{
					Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
					Port:          443,
					Hostname:      "hostname2",
					Weight:        50,
					ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
				},
			},
		},
	}

	err = mng.SetState(state)
	c.Assert(err, NoErr)

	c.Assert(withdrawTime.IsZero(), IsFalse)
	c.Assert(serviceTime.IsZero(), IsFalse)
	c.Assert(int(serviceTime.Sub(withdrawTime)/time.Millisecond), GreaterThan, 49)

	// reset timers to check delat during shutdown.
	serviceTime = time.Now()
	withdrawTime = serviceTime

	// 3. Shutdown.
	mng.Shutdown()
	c.Assert(int(serviceTime.Sub(withdrawTime)/time.Millisecond), GreaterThan, 49)
}

// validate custom shutdown logic.
func (m *ManagerSuite) TestCustomShutdown(c *C) {
	addrTable := NewMockAddressTableWithState().(*MockAddressTableModule)

	modules, err := GetMockModules(&ManagerModules{
		AddressTable: addrTable,
	})

	// 1. validate that custom shutdown handler has been called when it's
	// provided.
	handlerCalled := false
	modules.ShutdownHandler = func(currentState *kglb_pb.DataPlaneState) error {
		handlerCalled = true
		return nil
	}

	c.Assert(err, IsNil)
	mng, err := NewManager(*modules)
	c.Assert(err, IsNil)

	err = mng.Shutdown()
	c.Assert(err, IsNil)
	c.Assert(handlerCalled, IsTrue)

	// 2. Validate error propagition from custom handler.
	modules.ShutdownHandler = func(currentState *kglb_pb.DataPlaneState) error {
		return fmt.Errorf("custom shutdown error")
	}
	mng, err = NewManager(*modules)
	c.Assert(err, IsNil)
	err = mng.Shutdown()
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "custom shutdown error")

	// 3. Validate provided current state.
	modules.ShutdownHandler = func(currentState *kglb_pb.DataPlaneState) error {
		c.Logf("state: %+v", currentState)
		c.Assert(len(currentState.GetLinkAddresses()), Equals, 2)
		return nil
	}
	mng, err = NewManager(*modules)
	c.Assert(err, IsNil)
	// apply state to have something inside current state.
	err = mng.SetState(&kglb_pb.DataPlaneState{
		LinkAddresses: []*kglb_pb.LinkAddress{
			{
				LinkName: "lo",
				Address: &kglb_pb.IP{
					Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"},
				},
			},
			{
				LinkName: "lo",
				Address: &kglb_pb.IP{
					Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"},
				},
			},
		},
	})
	c.Assert(err, IsNil)
	err = mng.Shutdown()
	c.Assert(err, IsNil)
	// 4. validate that default shutdown logic doesn't remove any addresses
	// when custom handler is provided (since custom handler doesn't clean up
	// anything, added ips should still be in the state).
	state, err := addrTable.List("lo")
	c.Assert(err, IsNil)
	c.Assert(len(state), Equals, 3) // 2 custom + 127.0.0.1 default.
}
