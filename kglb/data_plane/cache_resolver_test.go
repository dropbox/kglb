package data_plane

import (
	"net"

	. "gopkg.in/check.v1"

	kglb_pb "dropbox/proto/kglb"
	. "godropbox/gocheck2"
)

type DbxResolverSuite struct {
}

var _ = Suite(&DbxResolverSuite{})

func (m *DbxResolverSuite) TestServiceLookup(c *C) {
	resolver, err := NewCacheResolver()
	c.Assert(err, IsNil)

	// 1. default value since no cache.
	c.Assert(
		resolver.ServiceLookup(
			&kglb_pb.IpvsService{
				Attributes: &kglb_pb.IpvsService_TcpAttributes{
					TcpAttributes: &kglb_pb.IpvsTcpAttributes{
						Address: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{
								Ipv4: "10.0.0.1",
							},
						},
						Port: 443,
					},
				},
			}),
		Equals,
		defaultService)

	// 2. update cache.
	state := &kglb_pb.DataPlaneState{
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
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
						Port:     443,
						Weight:   80,
						Hostname: "hostname1",
					},
				},
			},
			{
				Name: "TestName2",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_FwmarkAttributes{
							FwmarkAttributes: &kglb_pb.IpvsFwmarkAttributes{
								Fwmark: 10,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					}}},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.10"}},
						Port:     443,
						Weight:   80,
						Hostname: "hostname2",
					},
				},
			},
			{
				Name: "TestName1_v6",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv6{Ipv6: "2620:100:6015:300::a27d:1286"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					}}},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv6{Ipv6: "2620:100:6015:300::a27d:1287"}},
						Port:     443,
						Weight:   80,
						Hostname: "hostname2",
					},
				},
			},
		},
	}

	resolver.UpdateCache(state)
	c.Assert(
		resolver.ServiceLookup(
			&kglb_pb.IpvsService{
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
			}),
		Equals,
		"TestName1")
	c.Assert(
		resolver.ServiceLookup(
			&kglb_pb.IpvsService{
				Attributes: &kglb_pb.IpvsService_FwmarkAttributes{
					FwmarkAttributes: &kglb_pb.IpvsFwmarkAttributes{
						Fwmark: 10,
					},
				},
			}),
		Equals,
		"TestName2")
}

func (m *DbxResolverSuite) TestLookup(c *C) {
	resolver, err := NewCacheResolver()
	c.Assert(err, NoErr)

	// 1. default value since no cache.
	_, err = resolver.Lookup("hostname1")
	c.Assert(err, NotNil)

	// 2. update cache.
	state := &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
				Name: "TestName1",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "127.0.0.1"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					}}},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
						Port:     8080,
						Weight:   80,
						Hostname: "hostname1",
					},
					{
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
						Port:     8080,
						Weight:   80,
						Hostname: "hostname2",
					},
				}},
			{
				Name: "TestName1_v6",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv6{Ipv6: "2620:100:6015:300::a27d:1286"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					}}},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv6{Ipv6: "2620:100:6015:300::a27d:1287"}},
						Port:     8080,
						Weight:   80,
						Hostname: "hostname1",
					},
					{
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv6{Ipv6: "2620:100:6015:300::a27d:1288"}},
						Port:     8080,
						Weight:   80,
						Hostname: "hostname2",
					},
				}},
		}}

	resolver.UpdateCache(state)

	res, err := resolver.Lookup("hostname1")
	c.Assert(err, NoErr)
	c.Assert(res, DeepEqualsPretty, &HostnameCacheEntry{
		IPv4: net.ParseIP("10.0.0.1"),
		IPv6: net.ParseIP("2620:100:6015:300::a27d:1287"),
	})

	res, err = resolver.Lookup("hostname2")
	c.Assert(err, NoErr)
	c.Assert(res, DeepEqualsPretty, &HostnameCacheEntry{
		IPv4: net.ParseIP("10.0.0.2"),
		IPv6: net.ParseIP("2620:100:6015:300::a27d:1288"),
	})
}

func (m *DbxResolverSuite) TestReverseLookup(c *C) {
	resolver, err := NewCacheResolver()
	c.Assert(err, NoErr)

	// 1. default value since no cache.
	res, err := resolver.ReverseLookup(net.ParseIP("10.0.0.1"))
	c.Assert(err, NoErr)
	c.Assert(res, Equals, defaultHostname)

	// 2. update cache.
	state := &kglb_pb.DataPlaneState{
		Balancers: []*kglb_pb.BalancerState{
			{
				Name: "TestName1",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_TcpAttributes{
							TcpAttributes: &kglb_pb.IpvsTcpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "127.0.0.1"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					}}},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
						Port:     8080,
						Weight:   80,
						Hostname: "hostname1",
					},
					{
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"}},
						Port:     8080,
						Weight:   80,
						Hostname: "hostname2",
					},
				}}}}
	resolver.UpdateCache(state)

	res, err = resolver.ReverseLookup(net.ParseIP("10.0.0.1"))
	c.Assert(err, NoErr)
	c.Assert(res, Equals, "hostname1")
	res, err = resolver.ReverseLookup(net.ParseIP("10.0.0.2"))
	c.Assert(err, NoErr)
	c.Assert(res, Equals, "hostname2")
}

func (m *DbxResolverSuite) TestCluster(c *C) {
	resolver, err := NewCacheResolver()
	c.Assert(err, NoErr)

	c.Assert(resolver.Cluster("sjc8a-ra2-452"), Equals, "sjc8a")
	c.Assert(resolver.Cluster("sjc"), Equals, defaultRealCluster)
	c.Assert(resolver.Cluster("sjc15b-ra2-452"), Equals, "sjc15b")
}

func (m *DbxResolverSuite) TestKeyByService(c *C) {
	resolver, err := NewCacheResolver()
	c.Assert(err, NoErr)

	c.Assert(resolver.keyByService(&kglb_pb.IpvsService{
		Attributes: &kglb_pb.IpvsService_TcpAttributes{
			TcpAttributes: &kglb_pb.IpvsTcpAttributes{
				Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "127.0.0.1"}},
				Port:    443,
			},
		},
		Scheduler: kglb_pb.IpvsService_RR,
	}), Equals, "tcp-127.0.0.1:443")

	c.Assert(resolver.keyByService(&kglb_pb.IpvsService{
		Attributes: &kglb_pb.IpvsService_UdpAttributes{
			UdpAttributes: &kglb_pb.IpvsUdpAttributes{
				Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "127.0.0.1"}},
				Port:    443,
			},
		},
		Scheduler: kglb_pb.IpvsService_RR,
	}), Equals, "udp-127.0.0.1:443")
}

// testing handling cache table in case of multiple services with the same vip:port
// but different proto (tcp vs udp).
func (m *DbxResolverSuite) TestMultiServices(c *C) {
	resolver, err := NewCacheResolver()
	c.Assert(err, IsNil)

	state := &kglb_pb.DataPlaneState{
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
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
						Port:     443,
						Weight:   80,
						Hostname: "hostname1",
					},
				},
			},
			{
				Name: "TestName2",
				LbService: &kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
					IpvsService: &kglb_pb.IpvsService{
						Attributes: &kglb_pb.IpvsService_UdpAttributes{
							UdpAttributes: &kglb_pb.IpvsUdpAttributes{
								Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
								Port:    443,
							}},
						Scheduler: kglb_pb.IpvsService_RR,
					}}},
				Upstreams: []*kglb_pb.UpstreamState{
					{
						Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
						Port:     443,
						Weight:   80,
						Hostname: "hostname1",
					},
				},
			},
		},
	}

	resolver.UpdateCache(state)
	c.Assert(
		resolver.ServiceLookup(
			&kglb_pb.IpvsService{
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
			}),
		Equals,
		"TestName1")
	c.Assert(
		resolver.ServiceLookup(
			&kglb_pb.IpvsService{
				Attributes: &kglb_pb.IpvsService_UdpAttributes{
					UdpAttributes: &kglb_pb.IpvsUdpAttributes{
						Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
						Port:    443,
					}},
				Scheduler: kglb_pb.IpvsService_RR,
			}),
		Equals,
		"TestName2")
}
