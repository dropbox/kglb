package control_plane

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	. "gopkg.in/check.v1"

	"dropbox/kglb/utils/discovery"
	"dropbox/kglb/utils/dns_resolver"
	"dropbox/kglb/utils/fwmark"
	"dropbox/kglb/utils/health_manager"
	pb "dropbox/proto/kglb"
	hc_pb "dropbox/proto/kglb/healthchecker"
	. "godropbox/gocheck2"
)

type BalancerSuite struct{}

var _ = Suite(&BalancerSuite{})

var dummyChecker = &hc_pb.HealthCheckerAttributes{
	Attributes: &hc_pb.HealthCheckerAttributes_Dummy{
		Dummy: &hc_pb.DummyCheckerAttributes{},
	},
}

func (s *BalancerSuite) TestBasicFlow(c *C) {
	dnsCache := map[string]*pb.IP{
		"test-host-1": &pb.IP{
			Address: &pb.IP_Ipv4{
				Ipv4: "10.10.10.1",
			},
		},
		"test-host-2": &pb.IP{
			Address: &pb.IP_Ipv4{
				Ipv4: "10.10.10.2",
			},
		},
	}

	balancerConfig := &pb.BalancerConfig{
		Name: c.TestName(),
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
			RiseCount:  2,
			FallCount:  1,
			IntervalMs: 100,
			Checker:    dummyChecker,
		},
		EnableFwmarks: false,
		UpstreamDiscovery: &pb.UpstreamDiscovery{
			Port: 80,
			Attributes: &pb.UpstreamDiscovery_StaticAttributes{
				StaticAttributes: &pb.StaticDiscoveryAttributes{
					Hosts: []string{"test-host-1"},
				},
			},
		},
		DynamicRouting: &pb.DynamicRouting{
			AnnounceLimitRatio: 0.9,
			Attributes: &pb.DynamicRouting_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"},
					},
					Prefixlen: 32,
				},
			},
		},
		WeightUp: 222,
	}

	updatesChan := make(chan *BalancerState, 10)
	params := BalancerParams{
		BalancerConfig:  balancerConfig,
		ResolverFactory: NewDiscoveryFactory(),
		CheckerFactory:  NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{}),
		DnsResolver:     dns_resolver.NewDnsResolverMock(dnsCache),
		UpdatesChan:     updatesChan,
		FwmarkManager:   fwmark.NewManager(5000, 10000),
	}

	balancer, err := NewBalancer(context.Background(), params)
	c.Assert(err, IsNil)

	// in initial state everything is unhealthy
	select {
	case state, ok := <-balancer.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(state, NotNil)
		c.Assert(state.InitialState, IsTrue)
		c.Assert(len(state.States), Equals, 1)
		c.Assert(state.States[0], DeepEqualsPretty, &pb.BalancerState{
			Name:      c.TestName(),
			LbService: balancerConfig.GetLbService(),
			Upstreams: []*pb.UpstreamState{
				&pb.UpstreamState{
					Hostname: "test-host-1",
					Port:     80,
					Address: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "10.10.10.1"},
					},
					// forward method.
					ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
					Weight:        0,
				},
			},
		})
		c.Assert(len(state.States[0].Upstreams), Equals, 1)
		c.Assert(state.States[0].Upstreams[0], DeepEqualsPretty, &pb.UpstreamState{
			Hostname: "test-host-1",
			Port:     80,
			Address: &pb.IP{
				Address: &pb.IP_Ipv4{Ipv4: "10.10.10.1"},
			},
			// forward method.
			ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
			Weight:        0,
		})
	case <-time.After(5 * time.Second):
		c.Log("fails to wait second update from balancer.")
		c.Fail()
	}

	// second update with healthy hosts.
	select {
	case state, ok := <-balancer.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(state, NotNil)
		c.Assert(len(state.States), Equals, 1)
		c.Assert(state.InitialState, IsFalse)
		c.Assert(state.States[0], DeepEqualsPretty, &pb.BalancerState{
			Name:      c.TestName(),
			LbService: balancerConfig.GetLbService(),
			Upstreams: []*pb.UpstreamState{
				&pb.UpstreamState{
					Hostname: "test-host-1",
					Port:     80,
					Address: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "10.10.10.1"},
					},
					// forward method.
					ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
					Weight:        222,
				},
			},
		})
		c.Assert(len(state.States[0].Upstreams), Equals, 1)
		c.Assert(state.States[0].Upstreams[0], DeepEqualsPretty, &pb.UpstreamState{
			Hostname: "test-host-1",
			Port:     80,
			Address: &pb.IP{
				Address: &pb.IP_Ipv4{Ipv4: "10.10.10.1"},
			},
			// forward method.
			ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
			Weight:        222,
		})
	case <-time.After(5 * time.Second):
		c.Log("fails to wait second update from balancer.")
		c.Fail()
	}

	// updating.
	balancerConfig = &pb.BalancerConfig{
		Name: c.TestName(),
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
			RiseCount:  2,
			FallCount:  1,
			IntervalMs: 100,
			Checker:    dummyChecker,
		},
		EnableFwmarks: false,
		UpstreamDiscovery: &pb.UpstreamDiscovery{
			Port: 80,
			Attributes: &pb.UpstreamDiscovery_StaticAttributes{
				StaticAttributes: &pb.StaticDiscoveryAttributes{
					Hosts: []string{
						"test-host-1",
						"test-host-2",
					},
				},
			},
		},
		DynamicRouting: &pb.DynamicRouting{
			AnnounceLimitRatio: 0.9,
			Attributes: &pb.DynamicRouting_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"},
					},
					Prefixlen: 32,
				},
			},
		},
		WeightUp: 222,
	}
	err = balancer.Update(balancerConfig)
	c.Assert(err, IsNil)

	// checking new states.
	select {
	case state, ok := <-balancer.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(state, NotNil)
		c.Assert(len(state.States), Equals, 1)
		c.Assert(state.States[0], DeepEqualsPretty, &pb.BalancerState{
			Name:      c.TestName(),
			LbService: balancerConfig.GetLbService(),
			Upstreams: []*pb.UpstreamState{
				&pb.UpstreamState{
					Hostname: "test-host-1",
					Port:     80,
					Address: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "10.10.10.1"},
					},
					// forward method.
					ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
					Weight:        222,
				},
				&pb.UpstreamState{
					Hostname: "test-host-2",
					Port:     80,
					Address: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "10.10.10.2"},
					},
					// forward method.
					ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
					Weight:        0,
				},
			},
		})
	case <-time.After(5 * time.Second):
		c.Log("fails to wait forth update from balancer.")
		c.Fail()
	}

	// in next update host should be healthy.
	select {
	case state, ok := <-balancer.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(state, NotNil)
		c.Assert(len(state.States), Equals, 1)
		c.Assert(state.States[0], DeepEqualsPretty, &pb.BalancerState{
			Name:      c.TestName(),
			LbService: balancerConfig.GetLbService(),
			Upstreams: []*pb.UpstreamState{
				&pb.UpstreamState{
					Hostname: "test-host-1",
					Port:     80,
					Address: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "10.10.10.1"},
					},
					// forward method.
					ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
					Weight:        222,
				},
				&pb.UpstreamState{
					Hostname: "test-host-2",
					Port:     80,
					Address: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "10.10.10.2"},
					},
					// forward method.
					ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
					Weight:        222,
				},
			},
		})
	case <-time.After(5 * time.Second):
		c.Log("fails to wait fives update from balancer.")
		c.Fail()
	}

	// check closing.
	balancer.Close()
	select {
	case _, ok := <-balancer.ctx.Done():
		c.Assert(ok, IsFalse)
	case <-time.After(3 * time.Second):
		c.Fatal("fails to wait closing balancer.")
	}

	// checking closing state of health manager.
	select {
	case _, ok := <-balancer.healthMng.Updates():
		c.Assert(ok, IsFalse)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait closing health manager.")
	}

	// checking closing state of resolver.
	select {
	case _, ok := <-balancer.resolver.Updates():
		c.Assert(ok, IsFalse)
	case <-time.After(3 * time.Second):
		c.Fatal("fails to wait closing discovery resolver.")
	}
}

// validate that the channel is signaled to perform retry of state generation.
func (s *BalancerSuite) TestFailedDnsResolutionChann(c *C) {
	updatesChan := make(chan *BalancerState, 10)
	dnsResolver := &dns_resolver.ResolverMock{
		ResolverFunc: func(hostname string, af pb.AddressFamily) (*pb.IP, error) {
			return nil, fmt.Errorf("fails to resolve: %s", hostname)
		},
	}

	balancerConfig := &pb.BalancerConfig{
		Name: c.TestName(),
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
			RiseCount:        2,
			FallCount:        1,
			IntervalMs:       100,
			Checker:          dummyChecker,
			ConcurrencyLimit: 10,
		},
		EnableFwmarks: false,
		UpstreamDiscovery: &pb.UpstreamDiscovery{
			Port: 80,
			Attributes: &pb.UpstreamDiscovery_StaticAttributes{
				StaticAttributes: &pb.StaticDiscoveryAttributes{
					Hosts: []string{"test-host-1"},
				},
			},
		},
		DynamicRouting: &pb.DynamicRouting{
			AnnounceLimitRatio: 0.9,
			Attributes: &pb.DynamicRouting_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"},
					},
					Prefixlen: 32,
				},
			},
		},
		WeightUp: 222,
	}

	params := BalancerParams{
		BalancerConfig:      balancerConfig,
		ResolverFactory:     NewDiscoveryFactory(),
		CheckerFactory:      NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{}),
		DnsResolver:         dnsResolver,
		UpdatesChan:         updatesChan,
		UpdateRetryWaitTime: 100 * time.Millisecond,
		FwmarkManager:       fwmark.NewManager(5000, 10000),
	}

	balancer, err := newBalancer(context.Background(), params)
	c.Assert(err, IsNil)

	// health manager state.
	healthMngState := health_manager.HealthManagerState{
		health_manager.HealthManagerEntry{
			HostPort: discovery.NewHostPort("my_host", 8080),
			Status:   health_manager.NewHealthStatusEntry(false),
		},
	}

	// try to apply state which should notify chan
	startTime := time.Now()
	balancer.updateState(healthMngState)

	select {
	case <-balancer.updatesConf:
		elapsed := time.Since(startTime)
		c.Assert(elapsed, GreaterThan, 99*time.Millisecond)
	case <-time.After(200 * time.Millisecond):
		c.Log("missed notification.")
		c.Fail()
	}
}

// validate whole flow of state generation in case of failed dns resolution.
func (s *BalancerSuite) TestFailedDnsResolution(c *C) {
	updatesChan := make(chan *BalancerState, 10)

	failResolution := uint32(1)
	dnsResolver := &dns_resolver.ResolverMock{
		ResolverFunc: func(hostname string, af pb.AddressFamily) (*pb.IP, error) {
			if atomic.LoadUint32(&failResolution) > 0 {
				return nil, fmt.Errorf("fails to resolve: %s", hostname)
			} else {
				return &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}}, nil
			}
		},
	}

	balancerConfig := &pb.BalancerConfig{
		Name: c.TestName(),
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
			RiseCount:  2,
			FallCount:  1,
			IntervalMs: 10,
			Checker:    dummyChecker,
		},
		EnableFwmarks: false,
		UpstreamDiscovery: &pb.UpstreamDiscovery{
			Port: 80,
			Attributes: &pb.UpstreamDiscovery_StaticAttributes{
				StaticAttributes: &pb.StaticDiscoveryAttributes{
					Hosts: []string{"test-host-1"},
				},
			},
		},
		DynamicRouting: &pb.DynamicRouting{
			AnnounceLimitRatio: 0.9,
			Attributes: &pb.DynamicRouting_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"},
					},
					Prefixlen: 32,
				},
			},
		},
		WeightUp: 222,
	}

	params := BalancerParams{
		BalancerConfig:      balancerConfig,
		ResolverFactory:     NewDiscoveryFactory(),
		CheckerFactory:      NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{}),
		DnsResolver:         dnsResolver,
		UpdatesChan:         updatesChan,
		UpdateRetryWaitTime: 100 * time.Millisecond,
		FwmarkManager:       fwmark.NewManager(5000, 10000),
	}

	balancer, err := NewBalancer(context.Background(), params)
	c.Assert(err, IsNil)
	defer balancer.Close()

	select {
	case <-updatesChan:
		c.Log("unexpected state from balancer.")
		c.Fail()
	case <-time.After(200 * time.Millisecond):
	}

	// fixing dns resolution.
	atomic.StoreUint32(&failResolution, 0)
	select {
	case <-updatesChan:
	case <-time.After(200 * time.Millisecond):
		c.Log("timeout, no state from balancer too long.")
		c.Fail()
	}
}

// Validate that balancer doesn't allow to update name, setup name or vip.
func (s *BalancerSuite) TestUpdatingNameVips(c *C) {
	balancerConfig := &pb.BalancerConfig{
		Name:      c.TestName(),
		SetupName: fmt.Sprintf("setup_%s", c.TestName()),
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
			RiseCount:        2,
			FallCount:        1,
			IntervalMs:       100,
			Checker:          dummyChecker,
			ConcurrencyLimit: 10,
		},
		EnableFwmarks: false,
		UpstreamDiscovery: &pb.UpstreamDiscovery{
			Port: 80,
			Attributes: &pb.UpstreamDiscovery_StaticAttributes{
				StaticAttributes: &pb.StaticDiscoveryAttributes{
					Hosts: []string{"test-host-1"},
				},
			},
		},
		DynamicRouting: &pb.DynamicRouting{
			AnnounceLimitRatio: 0.9,
			Attributes: &pb.DynamicRouting_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"},
					},
					Prefixlen: 32,
				},
			},
		},
		WeightUp: 222,
	}
	// balancer stats.
	updatesChan := make(chan *BalancerState, 10)
	params := BalancerParams{
		BalancerConfig:  balancerConfig,
		ResolverFactory: NewDiscoveryFactory(),
		CheckerFactory:  NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{}),
		DnsResolver:     dns_resolver.NewDnsResolverMock(map[string]*pb.IP{}),
		UpdatesChan:     updatesChan,
		FwmarkManager:   fwmark.NewManager(5000, 10000),
	}

	balancer, err := NewBalancer(context.Background(), params)
	c.Assert(err, IsNil)

	// trying to update name.
	err = balancer.Update(&pb.BalancerConfig{
		Name: "new name",
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
			RiseCount:        2,
			FallCount:        1,
			IntervalMs:       100,
			Checker:          dummyChecker,
			ConcurrencyLimit: 15,
		},
		EnableFwmarks: false,
		UpstreamDiscovery: &pb.UpstreamDiscovery{
			Port: 80,
			Attributes: &pb.UpstreamDiscovery_StaticAttributes{
				StaticAttributes: &pb.StaticDiscoveryAttributes{
					Hosts: []string{"test-host-1"},
				},
			},
		},
		DynamicRouting: &pb.DynamicRouting{
			AnnounceLimitRatio: 0.9,
			Attributes: &pb.DynamicRouting_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"},
					},
					Prefixlen: 32,
				},
			},
		},
	})
	c.Assert(err, NotNil)

	// trying to update setup name.
	err = balancer.Update(&pb.BalancerConfig{
		SetupName: "new setup name",
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
			RiseCount:        2,
			FallCount:        1,
			IntervalMs:       100,
			Checker:          dummyChecker,
			ConcurrencyLimit: 15,
		},
		EnableFwmarks: false,
		UpstreamDiscovery: &pb.UpstreamDiscovery{
			Port: 80,
			Attributes: &pb.UpstreamDiscovery_StaticAttributes{
				StaticAttributes: &pb.StaticDiscoveryAttributes{
					Hosts: []string{"test-host-1"},
				},
			},
		},
		DynamicRouting: &pb.DynamicRouting{
			AnnounceLimitRatio: 0.9,
			Attributes: &pb.DynamicRouting_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"},
					},
					Prefixlen: 32,
				},
			},
		},
	})
	c.Assert(err, NotNil)

	// updating vip.
	err = balancer.Update(&pb.BalancerConfig{
		Name: c.TestName(),
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_TcpAttributes{
						TcpAttributes: &pb.IpvsTcpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
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
			RiseCount:  2,
			FallCount:  1,
			IntervalMs: 100,
			Checker:    dummyChecker,
		},
		EnableFwmarks: false,
		UpstreamDiscovery: &pb.UpstreamDiscovery{
			Port: 80,
			Attributes: &pb.UpstreamDiscovery_StaticAttributes{
				StaticAttributes: &pb.StaticDiscoveryAttributes{
					Hosts: []string{"test-host-1"},
				},
			},
		},
	})
	c.Assert(err, NotNil)
}

// Validate that balancer doesn't allow to update name, setup name or vip.
func (s *BalancerSuite) TestUpdatingCustomWeightAndUpdate(c *C) {
	balancerConfig := &pb.BalancerConfig{
		Name:      c.TestName(),
		SetupName: fmt.Sprintf("setup_%s", c.TestName()),
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
			RiseCount:        2,
			FallCount:        1,
			IntervalMs:       100,
			Checker:          dummyChecker,
			ConcurrencyLimit: 10,
		},
		EnableFwmarks: false,
		UpstreamDiscovery: &pb.UpstreamDiscovery{
			Port: 80,
			Attributes: &pb.UpstreamDiscovery_StaticAttributes{
				StaticAttributes: &pb.StaticDiscoveryAttributes{
					Hosts: []string{"test-host-1"},
				},
			},
		},
		DynamicRouting: &pb.DynamicRouting{
			AnnounceLimitRatio: 0.9,
			Attributes: &pb.DynamicRouting_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"},
					},
					Prefixlen: 32,
				},
			},
		},
	}
	dnsResolver := dns_resolver.NewDnsResolverMock(map[string]*pb.IP{
		"test-host-1": &pb.IP{
			Address: &pb.IP_Ipv4{
				Ipv4: "10.10.10.1",
			},
		},
		"test-host-2": &pb.IP{
			Address: &pb.IP_Ipv4{
				Ipv4: "10.10.10.2",
			},
		},
	})
	updatesChan := make(chan *BalancerState, 10)
	params := BalancerParams{
		BalancerConfig:  balancerConfig,
		ResolverFactory: NewDiscoveryFactory(),
		CheckerFactory:  NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{}),
		DnsResolver:     dnsResolver,
		UpdatesChan:     updatesChan,
		FwmarkManager:   fwmark.NewManager(5000, 10000),
	}

	balancer, err := NewBalancer(context.Background(), params)
	c.Assert(err, IsNil)

	// skipping initial update.
	select {
	case <-balancer.Updates():
	case <-time.After(5 * time.Second):
		c.Log("fails to wait update from balancer.")
		c.Fail()
	}

	// second update with healthy hosts.
	select {
	case state, ok := <-balancer.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(state, NotNil)
		c.Assert(len(state.States), Equals, 1)
		c.Assert(state.States[0], DeepEqualsPretty, &pb.BalancerState{
			Name:      c.TestName(),
			LbService: balancerConfig.GetLbService(),
			Upstreams: []*pb.UpstreamState{
				&pb.UpstreamState{
					Hostname: "test-host-1",
					Port:     80,
					Address: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "10.10.10.1"},
					},
					// forward method.
					ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
					Weight:        1000,
				},
			},
		})
	case <-time.After(5 * time.Second):
		c.Log("fails to wait update from balancer.")
		c.Fail()
	}

	// trying to update weight.
	balancerConfig.WeightUp = 222
	err = balancer.Update(balancerConfig)
	c.Assert(err, IsNil)

	// second update with healthy hosts.
	select {
	case state, ok := <-balancer.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(state, NotNil)
		c.Assert(len(state.States), Equals, 1)
		c.Assert(state.States[0], DeepEqualsPretty, &pb.BalancerState{
			Name:      c.TestName(),
			LbService: balancerConfig.GetLbService(),
			Upstreams: []*pb.UpstreamState{
				&pb.UpstreamState{
					Hostname: "test-host-1",
					Port:     80,
					Address: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "10.10.10.1"},
					},
					// forward method.
					ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
					Weight:        222,
				},
			},
		})
	case <-time.After(5 * time.Second):
		c.Log("fails to wait update from balancer.")
		c.Fail()
	}

	// updating to default weight.
	balancerConfig.WeightUp = 0
	err = balancer.Update(balancerConfig)
	c.Assert(err, IsNil)

	select {
	case state, ok := <-balancer.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(state, NotNil)
		c.Assert(len(state.States), Equals, 1)
		c.Assert(state.States[0], DeepEqualsPretty, &pb.BalancerState{
			Name:      c.TestName(),
			LbService: balancerConfig.GetLbService(),
			Upstreams: []*pb.UpstreamState{
				&pb.UpstreamState{
					Hostname: "test-host-1",
					Port:     80,
					Address: &pb.IP{
						Address: &pb.IP_Ipv4{Ipv4: "10.10.10.1"},
					},
					// forward method.
					ForwardMethod: balancerConfig.GetUpstreamRouting().GetForwardMethod(),
					Weight:        DefaultWeightUp,
				},
			},
		})
	case <-time.After(5 * time.Second):
		c.Log("fails to wait update from balancer.")
		c.Fail()
	}
}
