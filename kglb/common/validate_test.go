package common

import (
	. "gopkg.in/check.v1"

	pb "dropbox/proto/kglb"
	hc_pb "dropbox/proto/kglb/healthchecker"
	"godropbox/errors"
)

type ConfigSuite struct{}

var _ = Suite(&ConfigSuite{})

var dummyChecker = &hc_pb.HealthCheckerAttributes{
	Attributes: &hc_pb.HealthCheckerAttributes_Dummy{
		Dummy: &hc_pb.DummyCheckerAttributes{},
	},
}

func (s *ConfigSuite) TestValidateConfiguration(c *C) {
	cfg := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name: "balancer-1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    80,
								},
							},
						},
					},
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  1,
					FallCount:  1,
					IntervalMs: 1000,
					Checker:    dummyChecker,
				},
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"host-1"},
						},
					},
				},
			},
			{
				Name: "balancer-1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    80,
								},
							},
						},
					},
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  1,
					FallCount:  1,
					IntervalMs: 1000,
					Checker:    dummyChecker,
				},
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"host-2"},
						},
					},
				},
			},
		},
	}

	err := ValidateControlPlaneConfig(cfg)
	c.Assert(err, NotNil)
}

func (s *ConfigSuite) TestDupBalancerAndVips(c *C) {
	// dup names with different vip:vport is allowed.
	cfg := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "balancer-1",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    80,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  1,
					FallCount:  1,
					IntervalMs: 1000,
					Checker:    dummyChecker,
				},
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"host-1"},
						},
					},
				},
				DynamicRouting: &pb.DynamicRouting{
					AnnounceLimitRatio: 0.0,
					Attributes: &pb.DynamicRouting_BgpAttributes{
						BgpAttributes: &pb.BgpRouteAttributes{
							LocalAsn:  1000,
							PeerAsn:   1000,
							Community: "10000:10000",
							Prefix: &pb.IP{
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
			{
				Name:      "balancer-1",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    443,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  1,
					FallCount:  1,
					IntervalMs: 1000,
					Checker:    dummyChecker,
				},
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"host-2"},
						},
					},
				},
				DynamicRouting: &pb.DynamicRouting{
					AnnounceLimitRatio: 0.0,
					Attributes: &pb.DynamicRouting_BgpAttributes{
						BgpAttributes: &pb.BgpRouteAttributes{
							LocalAsn:  1000,
							PeerAsn:   1000,
							Community: "10000:10000",
							Prefix: &pb.IP{
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}

	err := ValidateControlPlaneConfig(cfg)
	c.Assert(err, IsNil)

	// different name with the same vip:vport is not allowed.
	cfg = &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "balancer-1",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    80,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  1,
					FallCount:  1,
					IntervalMs: 1000,
					Checker:    dummyChecker,
				},
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"host-1"},
						},
					},
				},
				DynamicRouting: &pb.DynamicRouting{
					AnnounceLimitRatio: 0.0,
					Attributes: &pb.DynamicRouting_BgpAttributes{
						BgpAttributes: &pb.BgpRouteAttributes{
							LocalAsn:  1000,
							PeerAsn:   1000,
							Community: "10000:10000",
							Prefix: &pb.IP{
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
			{
				Name:      "balancer-2",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    80,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  1,
					FallCount:  1,
					IntervalMs: 1000,
					Checker:    dummyChecker,
				},
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"host-2"},
						},
					},
				},
				DynamicRouting: &pb.DynamicRouting{
					AnnounceLimitRatio: 0.0,
					Attributes: &pb.DynamicRouting_BgpAttributes{
						BgpAttributes: &pb.BgpRouteAttributes{
							LocalAsn:  1000,
							PeerAsn:   1000,
							Community: "10000:10000",
							Prefix: &pb.IP{
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}

	err = ValidateControlPlaneConfig(cfg)
	c.Assert(err, NotNil)
}

func (s *ConfigSuite) TestInconsistentEnabledFwmark(c *C) {
	// dup names with different vip:vport is allowed.
	cfg := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "balancer-1",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    80,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  1,
					FallCount:  1,
					IntervalMs: 1000,
					Checker:    dummyChecker,
				},
				EnableFwmarks: true,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"host-1"},
						},
					},
				},
				DynamicRouting: &pb.DynamicRouting{
					AnnounceLimitRatio: 0.0,
					Attributes: &pb.DynamicRouting_BgpAttributes{
						BgpAttributes: &pb.BgpRouteAttributes{
							LocalAsn:  1000,
							PeerAsn:   1000,
							Community: "10000:10000",
							Prefix: &pb.IP{
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
			{
				Name:      "balancer-2",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_UdpAttributes{
								UdpAttributes: &pb.IpvsUdpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    443,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  1,
					FallCount:  1,
					IntervalMs: 1000,
					Checker:    dummyChecker,
				},
				EnableFwmarks: true,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"host-2"},
						},
					},
				},
				DynamicRouting: &pb.DynamicRouting{
					AnnounceLimitRatio: 0.0,
					Attributes: &pb.DynamicRouting_BgpAttributes{
						BgpAttributes: &pb.BgpRouteAttributes{
							LocalAsn:  1000,
							PeerAsn:   1000,
							Community: "10000:10000",
							Prefix: &pb.IP{
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}

	err := ValidateControlPlaneConfig(cfg)
	c.Assert(err, IsNil)
	cfg.Balancers[0].EnableFwmarks = false
	err = ValidateControlPlaneConfig(cfg)
	c.Assert(err, NotNil)
}

func (s *ConfigSuite) TestValidateBalancer(c *C) {
	b := &pb.BalancerConfig{Name: ""}
	err := ValidateBalancer(b)
	c.Assert(errors.GetMessage(err), Equals, "Name cannot be empty")
}

func (s *ConfigSuite) TestValidateIpvsService(c *C) {}

func (s *ConfigSuite) TestValidateUpstreamDiscovery(c *C) {
	m := &pb.UpstreamDiscovery{
		Attributes: &pb.UpstreamDiscovery_StaticAttributes{
			StaticAttributes: &pb.StaticDiscoveryAttributes{
				Hosts: []string{},
			},
		},
	}
	err := ValidateUpstreamDiscovery(m)
	c.Assert(err, NotNil)
}

func (s *ConfigSuite) TestValidateLinkAddresses(c *C) {
	// 1. valid config.
	addrMap, err := ValidateLinkAddresses([]*pb.LinkAddress{
		{
			Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			LinkName: "lo",
		},
		{
			Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
			LinkName: "lo",
		},
	})
	c.Assert(err, IsNil)
	c.Assert(len(addrMap), Equals, 2)
	// 2. missed link name.
	_, err = ValidateLinkAddresses([]*pb.LinkAddress{
		{
			Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
		},
		{
			Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
			LinkName: "lo",
		},
	})
	c.Assert(err, NotNil)
	// 3. dup.
	_, err = ValidateLinkAddresses([]*pb.LinkAddress{
		{
			Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			LinkName: "lo",
		},
		{
			Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			LinkName: "lo",
		},
	})
	c.Assert(err, NotNil)
}

func (s *ConfigSuite) TestValidateDataPlaneState(c *C) {
	// valid config.
	err := ValidateDataPlaneState(&pb.DataPlaneState{
		LinkAddresses: []*pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			},
		},
	})
	c.Assert(err, IsNil)
}

func (s *ConfigSuite) TestValidateUpstreamCheckerSyslog(c *C) {
	// 1. validate syslog checker.
	err := ValidateUpstreamChecker(&pb.BalancerConfig{
		Name: "balancer-1",
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_TcpAttributes{
						TcpAttributes: &pb.IpvsTcpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Port:    80,
						},
					},
				},
			},
		},
		UpstreamChecker: &hc_pb.UpstreamChecker{
			RiseCount:  1,
			FallCount:  1,
			IntervalMs: 1000,
			Checker: &hc_pb.HealthCheckerAttributes{
				Attributes: &hc_pb.HealthCheckerAttributes_Syslog{
					Syslog: &hc_pb.SyslogCheckerAttributes{
						Port: 70000,
					},
				},
			},
		},
	})
	c.Assert(err, NotNil)
	err = ValidateUpstreamChecker(&pb.BalancerConfig{
		Name: "balancer-1",
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_TcpAttributes{
						TcpAttributes: &pb.IpvsTcpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Port:    80,
						},
					},
				},
			},
		},
		UpstreamChecker: &hc_pb.UpstreamChecker{
			RiseCount:  1,
			FallCount:  1,
			IntervalMs: 1000,
			Checker: &hc_pb.HealthCheckerAttributes{
				Attributes: &hc_pb.HealthCheckerAttributes_Syslog{
					Syslog: &hc_pb.SyslogCheckerAttributes{
						Port: 1024,
					},
				},
			},
		},
	})
	c.Assert(err, IsNil)
}

func (s *ConfigSuite) TestValidateUpstreamCheckerDns(c *C) {
	// 1. valid config.
	err := ValidateUpstreamChecker(&pb.BalancerConfig{
		Name: "balancer-1",
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_TcpAttributes{
						TcpAttributes: &pb.IpvsTcpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Port:    80,
						},
					},
				},
			},
		},
		UpstreamChecker: &hc_pb.UpstreamChecker{
			RiseCount:  1,
			FallCount:  1,
			IntervalMs: 1000,
			Checker: &hc_pb.HealthCheckerAttributes{
				Attributes: &hc_pb.HealthCheckerAttributes_Dns{
					Dns: &hc_pb.DnsCheckerAttributes{
						Protocol:    hc_pb.IPProtocol_TCP,
						QueryString: ".",
						QueryType:   hc_pb.DnsCheckerAttributes_NS,
					},
				},
			},
		},
	})
	c.Assert(err, IsNil)

	// 2. not matched protos.
	err = ValidateUpstreamChecker(&pb.BalancerConfig{
		Name: "balancer-1",
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_TcpAttributes{
						TcpAttributes: &pb.IpvsTcpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Port:    80,
						},
					},
				},
			},
		},
		UpstreamChecker: &hc_pb.UpstreamChecker{
			RiseCount:  1,
			FallCount:  1,
			IntervalMs: 1000,
			Checker: &hc_pb.HealthCheckerAttributes{
				Attributes: &hc_pb.HealthCheckerAttributes_Dns{
					Dns: &hc_pb.DnsCheckerAttributes{

						Protocol:    hc_pb.IPProtocol_UDP,
						QueryString: ".",
						QueryType:   hc_pb.DnsCheckerAttributes_NS,
					},
				},
			},
		},
	})
	c.Assert(err, IsNil)
	err = ValidateUpstreamChecker(&pb.BalancerConfig{
		Name: "balancer-1",
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_UdpAttributes{
						UdpAttributes: &pb.IpvsUdpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Port:    80,
						},
					},
				},
			},
		},
		UpstreamChecker: &hc_pb.UpstreamChecker{
			RiseCount:  1,
			FallCount:  1,
			IntervalMs: 1000,
			Checker: &hc_pb.HealthCheckerAttributes{
				Attributes: &hc_pb.HealthCheckerAttributes_Dns{
					Dns: &hc_pb.DnsCheckerAttributes{
						Protocol:    hc_pb.IPProtocol_TCP,
						QueryString: ".",
						QueryType:   hc_pb.DnsCheckerAttributes_NS,
					},
				},
			},
		},
	})
	c.Assert(err, IsNil)

	// 3. enabled fwmarks for tcp.
	err = ValidateUpstreamChecker(&pb.BalancerConfig{
		Name: "balancer-1",
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_TcpAttributes{
						TcpAttributes: &pb.IpvsTcpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Port:    80,
						},
					},
				},
			},
		},
		UpstreamChecker: &hc_pb.UpstreamChecker{
			RiseCount:  1,
			FallCount:  1,
			IntervalMs: 1000,
			Checker: &hc_pb.HealthCheckerAttributes{
				Attributes: &hc_pb.HealthCheckerAttributes_Dns{
					Dns: &hc_pb.DnsCheckerAttributes{
						Protocol:    hc_pb.IPProtocol_TCP,
						QueryString: ".",
						QueryType:   hc_pb.DnsCheckerAttributes_NS,
					},
				},
			},
		},
		EnableFwmarks: true,
	})
	c.Assert(err, IsNil)

	// 4. unsupported fwmark for udp
	err = ValidateUpstreamChecker(&pb.BalancerConfig{
		Name: "balancer-1",
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_TcpAttributes{
						TcpAttributes: &pb.IpvsTcpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Port:    80,
						},
					},
				},
			},
		},
		UpstreamChecker: &hc_pb.UpstreamChecker{
			RiseCount:  1,
			FallCount:  1,
			IntervalMs: 1000,
			Checker: &hc_pb.HealthCheckerAttributes{
				Attributes: &hc_pb.HealthCheckerAttributes_Dns{
					Dns: &hc_pb.DnsCheckerAttributes{
						Protocol:    hc_pb.IPProtocol_UDP,
						QueryString: ".",
						QueryType:   hc_pb.DnsCheckerAttributes_NS,
					},
				},
			},
		},
		EnableFwmarks: true,
	})
	c.Assert(err, NotNil)

}
