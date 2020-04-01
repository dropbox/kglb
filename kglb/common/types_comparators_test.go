package common

import (
	. "godropbox/gocheck2"

	. "gopkg.in/check.v1"

	kglb_pb "dropbox/proto/kglb"
)

type ComparatorsSuite struct {
}

var _ = Suite(&ComparatorsSuite{})

func (m *ComparatorsSuite) TestUpstreams(c *C) {
	diff := CompareUpstreamState(
		[]*kglb_pb.UpstreamState{
			{
				Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
				Port:          443,
				Hostname:      "hostname1",
				Weight:        50,
				ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
			},
		},
		[]*kglb_pb.UpstreamState{
			{
				Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
				Port:          443,
				Hostname:      "hostname1",
				Weight:        90,
				ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
			},
		},
	)
	c.Assert(len(diff.Added), Equals, 0)
	c.Assert(len(diff.Deleted), Equals, 0)
	c.Assert(len(diff.Unchanged), Equals, 0)
	c.Assert(len(diff.Changed), Equals, 1)
	c.Assert(
		diff.Changed[0].NewItem.(*kglb_pb.UpstreamState),
		DeepEqualsPretty,
		&kglb_pb.UpstreamState{
			Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
			Port:          443,
			Hostname:      "hostname1",
			Weight:        90,
			ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
		})
}

func (m *ComparatorsSuite) TestLoadBalancerService(c *C) {
	diff := CompareLoadBalancerService(
		[]*kglb_pb.LoadBalancerService{
			{
				Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
								Port:    443,
							},
						},
						Scheduler: kglb_pb.IpvsService_RR,
					},
				},
			},
		},
		[]*kglb_pb.LoadBalancerService{
			{
				Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					},
				},
			},
		},
	)
	c.Assert(len(diff.Added), Equals, 0)
	c.Assert(len(diff.Deleted), Equals, 0)
	c.Assert(len(diff.Unchanged), Equals, 1)

	diff = CompareLoadBalancerService(
		[]*kglb_pb.LoadBalancerService{
			{
				Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
								Port:    443,
							},
						},
						Scheduler: kglb_pb.IpvsService_RR,
					},
				},
			},
		},
		[]*kglb_pb.LoadBalancerService{
			{
				Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					},
				},
			},
		},
	)
	c.Assert(len(diff.Added), Equals, 1)
	c.Assert(len(diff.Deleted), Equals, 1)
	c.Assert(len(diff.Unchanged), Equals, 0)
	c.Assert(len(diff.Changed), Equals, 0)
}

func (m *ComparatorsSuite) TestBalancersStates(c *C) {
	diff := CompareBalancerState(
		[]*kglb_pb.BalancerState{
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
		[]*kglb_pb.BalancerState{
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
	)
	c.Assert(len(diff.Added), Equals, 0)
	c.Assert(len(diff.Deleted), Equals, 0)
	c.Assert(len(diff.Unchanged), Equals, 0)
	c.Assert(len(diff.Changed), Equals, 1)

	diff = CompareBalancerState(
		[]*kglb_pb.BalancerState{
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
		[]*kglb_pb.BalancerState{
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
						Hostname:      "hostname2",
						Weight:        50,
						ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
					},
				},
			},
		},
	)
	c.Assert(len(diff.Added), Equals, 0)
	c.Assert(len(diff.Deleted), Equals, 0)
	c.Assert(len(diff.Unchanged), Equals, 1)
	c.Assert(len(diff.Changed), Equals, 1)
}

func (m *ComparatorsSuite) TestDataPlaneStateEqual(c *C) {
	isEqual := DataPlaneStateComparable.Equal(
		&kglb_pb.DataPlaneState{
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
							Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
							Port:          443,
							Hostname:      "hostname2",
							Weight:        50,
							ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
						},
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
			LinkAddresses: []*kglb_pb.LinkAddress{
				{
					LinkName: "lo",
					Address: &kglb_pb.IP{
						Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"},
					},
				},
			}},
		&kglb_pb.DataPlaneState{
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
			LinkAddresses: []*kglb_pb.LinkAddress{
				{
					LinkName: "lo",
					Address: &kglb_pb.IP{
						Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"},
					},
				},
			},
		})
	c.Assert(isEqual, IsTrue)
}

func (m *ComparatorsSuite) TestLinkAddress(c *C) {
	diff := CompareLocalLinkAddresses(
		[]*kglb_pb.LinkAddress{
			{
				LinkName: "link1",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
			},
			{
				LinkName: "link2",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
			},
		},
		[]*kglb_pb.LinkAddress{
			{
				LinkName: "link1",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
			},
		},
	)
	c.Assert(len(diff.Added), Equals, 0)
	c.Assert(len(diff.Deleted), Equals, 1)
	c.Assert(len(diff.Unchanged), Equals, 1)
	c.Assert(len(diff.Changed), Equals, 0)
	c.Assert(diff.IsChanged(), IsTrue)

	diff = CompareLocalLinkAddresses(
		[]*kglb_pb.LinkAddress{
			{
				LinkName: "old_link",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
			},
		},
		[]*kglb_pb.LinkAddress{
			{
				LinkName: "new_link",
				Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
			},
		},
	)
	c.Assert(len(diff.Added), Equals, 1)
	c.Assert(len(diff.Deleted), Equals, 1)
	c.Assert(len(diff.Unchanged), Equals, 0)
	c.Assert(len(diff.Changed), Equals, 0)
	c.Assert(diff.IsChanged(), IsTrue)
	c.Assert(
		diff.Deleted[0].(*kglb_pb.LinkAddress),
		DeepEqualsPretty,
		&kglb_pb.LinkAddress{
			LinkName: "old_link",
			Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
		})
	c.Assert(
		diff.Added[0].(*kglb_pb.LinkAddress),
		DeepEqualsPretty,
		&kglb_pb.LinkAddress{
			LinkName: "new_link",
			Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
		})
}

func (m *ComparatorsSuite) TestBgpRouting(c *C) {
	diff := CompareDynamicRouting(
		[]*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  1000,
						PeerAsn:   1000,
						Community: "10000:10000,20000:20000",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
		[]*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  1000,
						PeerAsn:   1000,
						Community: "10000:10000 20000:20000",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
	)
	c.Assert(len(diff.Added), Equals, 0)
	c.Assert(len(diff.Deleted), Equals, 0)
	c.Assert(len(diff.Unchanged), Equals, 1)
	c.Assert(len(diff.Changed), Equals, 0)

	diff = CompareDynamicRouting(
		[]*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  1000,
						PeerAsn:   1000,
						Community: "10000:10000 20000:20000",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
		[]*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  1000,
						PeerAsn:   1000,
						Community: "10000:10000,20000:20000",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
	)
	c.Assert(len(diff.Added), Equals, 0)
	c.Assert(len(diff.Deleted), Equals, 0)
	c.Assert(len(diff.Unchanged), Equals, 1)
	c.Assert(len(diff.Changed), Equals, 0)

	diff = CompareDynamicRouting(
		[]*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  1000,
						PeerAsn:   1000,
						Community: "10000:10000 20000:20000",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"},
						},
						Prefixlen: 32,
					},
				},
			},
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  1000,
						PeerAsn:   1000,
						Community: "10000:10000 20000:20000",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.2"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
		[]*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  1000,
						PeerAsn:   1000,
						Community: "10000:10000,20000:20000",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"},
						},
						Prefixlen: 32,
					},
				},
			},
		},
	)
	c.Assert(len(diff.Added), Equals, 0)
	c.Assert(len(diff.Deleted), Equals, 1)
	c.Assert(len(diff.Unchanged), Equals, 1)
	c.Assert(len(diff.Changed), Equals, 0)
}
