package control_plane

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"sync/atomic"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	. "gopkg.in/check.v1"

	"dropbox/kglb/common"
	"dropbox/kglb/utils/dns_resolver"
	"dropbox/kglb/utils/fwmark"
	pb "dropbox/proto/kglb"
	hc_pb "dropbox/proto/kglb/healthchecker"
	. "godropbox/gocheck2"
)

type mockDpClient struct {
	setFunc func(state *pb.DataPlaneState) error
}

func (c mockDpClient) Set(state *pb.DataPlaneState) error {
	if c.setFunc == nil {
		panic("setFunc is not implemented")
	}
	return c.setFunc(state)
}

type ServicerSuite struct {
	modules ServicerModules
	tmpDir  string
}

var _ = Suite(&ServicerSuite{})

func (s *ServicerSuite) SetUpTest(c *C) {
	// temporary folder.
	s.tmpDir = c.MkDir()

	// modules.
	dnsResolver := dns_resolver.NewDnsResolverMock(map[string]*pb.IP{})

	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			return nil
		},
	}

	checkerFactory := NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{})

	discoveryFactory := NewDiscoveryFactory()

	configLoader := newMockConfigLoader()

	s.modules = ServicerModules{
		DnsResolver:      dnsResolver,
		CheckerFactory:   checkerFactory,
		DiscoveryFactory: discoveryFactory,
		DataPlaneClient:  client,
		ConfigLoader:     configLoader,
		// No op metric manager.
		FwmarkManager: fwmark.NewManager(5000, 10000),
	}

}

func (s *ServicerSuite) TestMissedBgp(c *C) {
	dnsData := make(map[string]*pb.IP)
	dnsData["test-host-1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}}
	dnsData["test-host-2"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.2"}}
	dnsResolver := dns_resolver.NewDnsResolverMock(dnsData)

	testConfig := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-1",
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
					IntervalMs: 1,
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
				},
			},
		},
	}

	expectedDpState := &pb.DataPlaneState{
		Balancers: []*pb.BalancerState{
			{
				Name: "test-balancer-1",
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
				Upstreams: []*pb.UpstreamState{
					{
						Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}},
						Hostname: "test-host-1",
						Port:     80,
						Weight:   1000,
					},
				},
			},
		},
		LinkAddresses: []*pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			},
		},
	}

	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			return nil
		},
	}

	checkerFactory := NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{})
	discoveryFactory := NewDiscoveryFactory()

	content := proto.MarshalTextString(testConfig)
	tmpfile, err := ioutil.TempFile(s.tmpDir, "")
	c.Assert(err, NoErr)
	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, NoErr)

	configLoader := newMockConfigLoader()
	configLoader.configChan <- testConfig

	servicerModules := ServicerModules{
		DnsResolver:      dnsResolver,
		CheckerFactory:   checkerFactory,
		DiscoveryFactory: discoveryFactory,
		DataPlaneClient:  client,
		ConfigLoader:     configLoader,
		// No op metric manager.
		FwmarkManager: fwmark.NewManager(5000, 10000),
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	servicer, err := NewControlPlaneServicer(
		ctx,
		servicerModules,
		time.Millisecond)
	c.Assert(err, NoErr)

	for i := 0; i < 10; i++ {
		state, err := servicer.GetConfiguration(context.Background(), &types.Empty{})
		c.Assert(err, NoErr)
		if state.String() == expectedDpState.String() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	state, err := servicer.GetConfiguration(context.Background(), &types.Empty{})
	c.Assert(err, NoErr)
	c.Assert(state, DeepEqualsPretty, expectedDpState)

	// closing control plane.
	cancelFunc()
	select {
	case _, ok := <-servicer.balancers["test-balancer-1-172.0.0.1:80-tcp"].healthMng.Updates():
		c.Assert(ok, IsFalse)
	case <-time.After(5 * time.Second):
		c.Log("fails to wait closing balancer and its health manager.")
		c.Fail()
	}
}

func (s *ServicerSuite) TestProcessServices(c *C) {
	dnsData := make(map[string]*pb.IP)
	dnsData["test-host-1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}}
	dnsData["test-host-2"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.2"}}
	dnsResolver := dns_resolver.NewDnsResolverMock(dnsData)

	testConfig := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-1",
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
					IntervalMs: 1,
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
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}

	expectedDpState := &pb.DataPlaneState{
		Balancers: []*pb.BalancerState{
			{
				Name: "test-balancer-1",
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
				Upstreams: []*pb.UpstreamState{
					{
						Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}},
						Hostname: "test-host-1",
						Port:     80,
						Weight:   1000,
					},
				},
			},
		},
		DynamicRoutes: []*pb.DynamicRoute{
			{
				Attributes: &pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &pb.BgpRouteAttributes{
						LocalAsn:  1000,
						PeerAsn:   1000,
						Community: "10000:10000",
						Prefix:    &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
						Prefixlen: 32,
					},
				},
			},
		},
		LinkAddresses: []*pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			},
		},
	}

	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			return nil
		},
	}

	checkerFactory := NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{})

	discoveryFactory := NewDiscoveryFactory()

	c.Assert(common.ValidateControlPlaneConfig(testConfig), NoErr)

	content := proto.MarshalTextString(testConfig)
	tmpfile, err := ioutil.TempFile(s.tmpDir, "")
	c.Assert(err, NoErr)
	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, NoErr)

	configLoader := newMockConfigLoader()
	configLoader.configChan <- testConfig

	servicerModules := ServicerModules{
		DnsResolver:      dnsResolver,
		CheckerFactory:   checkerFactory,
		DiscoveryFactory: discoveryFactory,
		DataPlaneClient:  client,
		ConfigLoader:     configLoader,
		// No op metric manager.
		FwmarkManager: fwmark.NewManager(5000, 10000),
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	servicer, err := NewControlPlaneServicer(
		ctx,
		servicerModules,
		time.Millisecond)
	c.Assert(err, NoErr)

	for i := 0; i < 10; i++ {
		state, err := servicer.GetConfiguration(context.Background(), &types.Empty{})
		c.Assert(err, NoErr)
		if state.String() == expectedDpState.String() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	state, err := servicer.GetConfiguration(context.Background(), &types.Empty{})
	c.Assert(err, NoErr)
	c.Assert(state, DeepEqualsPretty, expectedDpState)

	// closing control plane.
	cancelFunc()
	select {
	case _, ok := <-servicer.balancers["test-balancer-1-172.0.0.1:80-tcp"].healthMng.Updates():
		c.Assert(ok, IsFalse)
	case <-time.After(5 * time.Second):
		c.Log("fails to wait closing balancer and its health manager.")
		c.Fail()
	}
}

// Setups two real servers and validates that they are properly health checked
// and generated balancer state is correct.
func (s *ServicerSuite) TestBalancerState(c *C) {
	// 1. create backends.
	failReal1 := uint32(0)
	srv1 := NewBackendWithCustomAddr(
		c,
		"127.1.0.1:15678",
		func(writer http.ResponseWriter, req *http.Request) {
			if atomic.LoadUint32(&failReal1) > 0 {
				writer.WriteHeader(http.StatusInternalServerError)
			} else {
				writer.WriteHeader(http.StatusOK)
			}
		})
	defer srv1.Close()

	failReal2 := uint32(0)
	srv2 := NewBackendWithCustomAddr(
		c,
		"127.2.0.1:15678",
		func(writer http.ResponseWriter, req *http.Request) {
			if atomic.LoadUint32(&failReal2) > 0 {
				writer.WriteHeader(http.StatusInternalServerError)
			} else {
				writer.WriteHeader(http.StatusOK)
			}
		})
	defer srv2.Close()

	// 2. create control_plane config.
	testConfig := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-1",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    15678,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  3,
					FallCount:  1,
					IntervalMs: 10,
					Checker: &hc_pb.HealthCheckerAttributes{
						Attributes: &hc_pb.HealthCheckerAttributes_Http{
							Http: &hc_pb.HttpCheckerAttributes{
								Scheme: "http",
								Uri:    "/",
								Codes: []uint32{
									http.StatusOK,
								},
							},
						},
					},
				},
				EnableFwmarks: false,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 15678,
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{
								"127.1.0.1",
								"127.2.0.1",
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
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}

	// channel to catch dp state.
	dpStateChan := make(chan *pb.DataPlaneState, 10)
	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			dpStateChan <- state
			return nil
		},
	}

	dnsData := make(map[string]*pb.IP)
	dnsData["127.1.0.1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "127.1.0.1"}}
	dnsData["127.2.0.1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "127.2.0.1"}}
	dnsResolver := dns_resolver.NewDnsResolverMock(dnsData)

	checkerFactory := NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{})

	discoveryFactory := NewDiscoveryFactory()

	c.Assert(common.ValidateControlPlaneConfig(testConfig), NoErr)

	content := proto.MarshalTextString(testConfig)
	tmpfile, err := ioutil.TempFile(s.tmpDir, "")
	c.Assert(err, NoErr)
	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, NoErr)

	configLoader := newMockConfigLoader()
	configLoader.configChan <- testConfig

	servicerModules := ServicerModules{
		DnsResolver:      dnsResolver,
		CheckerFactory:   checkerFactory,
		DiscoveryFactory: discoveryFactory,
		DataPlaneClient:  client,
		ConfigLoader:     configLoader,
		FwmarkManager:    fwmark.NewManager(5000, 10000),
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	_, err = NewControlPlaneServicer(
		ctx,
		servicerModules,
		time.Minute)
	c.Assert(err, NoErr)

	// step 1: both backends are unhealthy at the beginning.
	select {
	case state, ok := <-dpStateChan:
		c.Assert(ok, IsTrue)
		// waiting healthy state of all backends.
		balancers := state.GetBalancers()
		c.Assert(len(balancers), Equals, 1)

		upstreams := balancers[0].GetUpstreams()
		c.Assert(len(upstreams), Equals, 2)

		c.Assert(int(upstreams[0].GetWeight()), Equals, 0)
		c.Assert(int(upstreams[1].GetWeight()), Equals, 0)
		c.Assert(len(state.GetDynamicRoutes()), Equals, 0)
	case <-time.After(10 * time.Second):
		c.Log("fails to wait dp state, step1")
		c.Fail()
		return
	}

	// step 2: both backends are alive.
attempts:
	for i := 0; i < 3; i++ {
		select {
		case state, ok := <-dpStateChan:
			c.Assert(ok, IsTrue)
			// waiting healthy state of all backends.
			balancers := state.GetBalancers()
			c.Assert(len(balancers), Equals, 1)

			upstreams := balancers[0].GetUpstreams()
			c.Assert(len(upstreams), Equals, 2)

			if int(upstreams[0].GetWeight()) != 1000 ||
				int(upstreams[1].GetWeight()) != 1000 {

				if i == 2 {
					c.Log("fails to wait health state of both upstreams.")
					c.Fail()
				}
				continue
			}

			c.Assert(int(upstreams[0].GetWeight()), Equals, 1000)
			c.Assert(int(upstreams[1].GetWeight()), Equals, 1000)

			c.Log("failing one of backend, attempt: ", i)
			atomic.StoreUint32(&failReal1, 1)
			break attempts
		case <-time.After(10 * time.Second):
			c.Log("fails to wait dp state, step1")
			c.Fail()
			return
		}
	}

	// step 3: one of backend is unhealthy.
	select {
	case state, ok := <-dpStateChan:
		c.Assert(ok, IsTrue)
		// waiting healthy state of all backends.
		balancers := state.GetBalancers()
		c.Assert(len(balancers), Equals, 1)

		upstreams := balancers[0].GetUpstreams()
		c.Assert(len(upstreams), Equals, 2)

		if upstreams[0].Hostname == "127.1.0.1" {
			c.Assert(int(upstreams[0].GetWeight()), Equals, 0)
			c.Assert(int(upstreams[1].GetWeight()), Equals, 1000)
		} else {
			c.Assert(int(upstreams[0].GetWeight()), Equals, 1000)
			c.Assert(int(upstreams[1].GetWeight()), Equals, 0)
		}

		c.Log("making backend as healthy again.")
		atomic.StoreUint32(&failReal1, 0)
	case <-time.After(10 * time.Second):
		c.Log("fails to wait dp state, step2")
		c.Fail()
		return
	}

	// step 4: making backend is healthy again.
	select {
	case state, ok := <-dpStateChan:
		c.Assert(ok, IsTrue)
		balancers := state.GetBalancers()
		c.Assert(len(balancers), Equals, 1)

		upstreams := balancers[0].GetUpstreams()
		c.Assert(len(upstreams), Equals, 2)

		c.Assert(int(upstreams[0].GetWeight()), Equals, 1000)
		c.Assert(int(upstreams[1].GetWeight()), Equals, 1000)

		c.Log("failing one of backend")
		atomic.StoreUint32(&failReal1, 1)
	case <-time.After(10 * time.Second):
		c.Log("fails to wait dp state, step1")
		c.Fail()
		return
	}
}

// Validates that control plan doesn't advertise route it balancer doesn't
// have any upstreams.
func (s *ServicerSuite) TestBalancerWithoutUpstreams(c *C) {
	dnsData := make(map[string]*pb.IP)
	dnsData["test-host-1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}}
	dnsData["test-host-2"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.2"}}
	dnsResolver := dns_resolver.NewDnsResolverMock(dnsData)

	testConfig := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-1",
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
					IntervalMs: 1,
					Checker:    dummyChecker,
				},
				EnableFwmarks: false,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 80,
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{},
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
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}

	expectedDpState := &pb.DataPlaneState{
		Balancers: []*pb.BalancerState{
			{
				Name: "test-balancer-1",
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
				Upstreams: []*pb.UpstreamState{},
			},
		},
		LinkAddresses: []*pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			},
		},
	}

	clientChan := make(chan *pb.DataPlaneState, 1)
	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			clientChan <- state
			return nil
		},
	}

	checkerFactory := NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{})

	discoveryFactory := NewDiscoveryFactory()

	content := proto.MarshalTextString(testConfig)
	tmpfile, err := ioutil.TempFile(s.tmpDir, "")
	c.Assert(err, NoErr)
	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, NoErr)

	configLoader := newMockConfigLoader()
	configLoader.configChan <- testConfig

	servicerModules := ServicerModules{
		DnsResolver:      dnsResolver,
		CheckerFactory:   checkerFactory,
		DiscoveryFactory: discoveryFactory,
		DataPlaneClient:  client,
		ConfigLoader:     configLoader,
		// No op metric manager.
		FwmarkManager: fwmark.NewManager(5000, 10000),
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	servicer, err := newControlPlaneServicer(
		ctx,
		servicerModules,
		time.Millisecond)
	c.Assert(err, NoErr)
	c.Assert(servicer.initialState, IsTrue)

	// do not expect call of callback since there is no running event loop in
	// servicer.
	servicer.modules.AfterInitHandler = func() {
		c.Log("unexpected call of AfterInitHandler()")
		c.Fail()
	}

	servicer.updateConfig(testConfig)
	servicer.balancers["test-balancer-1-172.0.0.1:80-tcp"].state.Store(&BalancerState{
		States: []*pb.BalancerState{
			{
				Name: "test-balancer-1",
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
				Upstreams: []*pb.UpstreamState{},
			},
		},
		AliveRatio:   0,
		InitialState: true,
	})

	state, err := servicer.GenerateDataPlaneState()
	c.Assert(err, IsNil)
	c.Assert(servicer.initialState, IsFalse)

	// dynamic routes should not be there even AliveRatio is positive since
	// balancer still in init state.
	c.Assert(len(state.GetDynamicRoutes()), Equals, 0)
	c.Assert(state, DeepEqualsPretty, expectedDpState)

	servicer.updateConfig(testConfig)
	servicer.balancers["test-balancer-1-172.0.0.1:80-tcp"].state.Store(&BalancerState{
		States: []*pb.BalancerState{
			{
				Name: "test-balancer-1",
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
				Upstreams: []*pb.UpstreamState{},
			},
		},
		AliveRatio:   1,
		InitialState: true,
	})

	state, err = servicer.GenerateDataPlaneState()
	c.Assert(err, IsNil)
	c.Assert(servicer.initialState, IsFalse)

	// dynamic routes should not be there.
	c.Assert(len(state.GetDynamicRoutes()), Equals, 0)
	c.Assert(state, DeepEqualsPretty, expectedDpState)
}

// Validates that in fallback mode control plane announce bgp route.
func (s *ServicerSuite) TestFallbackMode(c *C) {
	// 1. create backends.
	failReal1 := uint32(0)
	srv1 := NewBackendWithCustomAddr(
		c,
		"127.1.0.1:15678",
		func(writer http.ResponseWriter, req *http.Request) {
			if atomic.LoadUint32(&failReal1) > 0 {
				writer.WriteHeader(http.StatusInternalServerError)
			} else {
				writer.WriteHeader(http.StatusOK)
			}
		})
	defer srv1.Close()

	failReal2 := uint32(0)
	srv2 := NewBackendWithCustomAddr(
		c,
		"127.2.0.1:15678",
		func(writer http.ResponseWriter, req *http.Request) {
			if atomic.LoadUint32(&failReal2) > 0 {
				writer.WriteHeader(http.StatusInternalServerError)
			} else {
				writer.WriteHeader(http.StatusOK)
			}
		})
	defer srv2.Close()

	// 2. create control_plane config.
	testConfig := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-1",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    15678,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  3,
					FallCount:  1,
					IntervalMs: 10,
					Checker: &hc_pb.HealthCheckerAttributes{
						Attributes: &hc_pb.HealthCheckerAttributes_Http{
							Http: &hc_pb.HttpCheckerAttributes{
								Scheme: "http",
								Uri:    "/",
								Codes: []uint32{
									http.StatusOK,
								},
							},
						},
					},
				},
				EnableFwmarks: false,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 15678,
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{
								"127.1.0.1",
								"127.2.0.1",
							},
						},
					},
				},
				DynamicRouting: &pb.DynamicRouting{
					AnnounceLimitRatio: 0.5, // one of two will simulate unhealthy state.
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

	// channel to catch dp state.
	dpStateChan := make(chan *pb.DataPlaneState, 10)
	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			dpStateChan <- state
			return nil
		},
	}

	dnsData := make(map[string]*pb.IP)
	dnsData["127.1.0.1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "127.1.0.1"}}
	dnsData["127.2.0.1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "127.2.0.1"}}
	dnsResolver := dns_resolver.NewDnsResolverMock(dnsData)

	checkerFactory := NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{})

	discoveryFactory := NewDiscoveryFactory()

	c.Assert(common.ValidateControlPlaneConfig(testConfig), NoErr)

	content := proto.MarshalTextString(testConfig)
	tmpfile, err := ioutil.TempFile(s.tmpDir, "")
	c.Assert(err, NoErr)
	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, NoErr)

	configLoader := newMockConfigLoader()
	configLoader.configChan <- testConfig

	servicerModules := ServicerModules{
		DnsResolver:      dnsResolver,
		CheckerFactory:   checkerFactory,
		DiscoveryFactory: discoveryFactory,
		DataPlaneClient:  client,
		ConfigLoader:     configLoader,
		FwmarkManager:    fwmark.NewManager(5000, 10000),
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	_, err = NewControlPlaneServicer(
		ctx,
		servicerModules,
		time.Minute)
	c.Assert(err, NoErr)

	// step 1: both backends are unhealthy at the beginning.
	select {
	case state, ok := <-dpStateChan:
		c.Assert(ok, IsTrue)
		// waiting healthy state of all backends.
		balancers := state.GetBalancers()
		c.Assert(len(balancers), Equals, 1)

		upstreams := balancers[0].GetUpstreams()
		c.Assert(len(upstreams), Equals, 2)

		c.Assert(int(upstreams[0].GetWeight()), Equals, 0)
		c.Assert(int(upstreams[1].GetWeight()), Equals, 0)
		c.Assert(len(state.GetDynamicRoutes()), Equals, 0)
	case <-time.After(10 * time.Second):
		c.Log("fails to wait dp state, step1")
		c.Fail()
		return
	}

	// step 2: both backends are alive.
attempts:
	for i := 0; i < 3; i++ {
		select {
		case state, ok := <-dpStateChan:
			c.Assert(ok, IsTrue)
			// waiting healthy state of all backends.
			balancers := state.GetBalancers()
			c.Assert(len(balancers), Equals, 1)

			upstreams := balancers[0].GetUpstreams()
			c.Assert(len(upstreams), Equals, 2)

			if int(upstreams[0].GetWeight()) != 1000 ||
				int(upstreams[1].GetWeight()) != 1000 {

				if i == 2 {
					c.Log("fails to wait health state of both upstreams.")
					c.Fail()
				}
				continue
			}
			c.Assert(int(upstreams[0].GetWeight()), Equals, 1000)
			c.Assert(int(upstreams[1].GetWeight()), Equals, 1000)
			c.Assert(len(state.GetDynamicRoutes()), Equals, 1)
			c.Log("failing one of backend, attempt: ", i)
			atomic.StoreUint32(&failReal1, 1)
			break attempts
		case <-time.After(10 * time.Second):
			c.Log("fails to wait dp state, step1")
			c.Fail()
			return
		}
	}

	// step 3: one of backend is unhealthy.
	select {
	case state, ok := <-dpStateChan:
		c.Assert(ok, IsTrue)
		// waiting healthy state of all backends.
		balancers := state.GetBalancers()
		c.Assert(len(balancers), Equals, 1)

		upstreams := balancers[0].GetUpstreams()
		c.Assert(len(upstreams), Equals, 2)

		if upstreams[0].Hostname == "127.1.0.1" {
			c.Assert(int(upstreams[0].GetWeight()), Equals, 0)
			c.Assert(int(upstreams[1].GetWeight()), Equals, 1000)
		} else {
			c.Assert(int(upstreams[0].GetWeight()), Equals, 1000)
			c.Assert(int(upstreams[1].GetWeight()), Equals, 0)
		}

		c.Assert(len(state.GetDynamicRoutes()), Equals, 1)
		c.Log("making backend as healthy again.")
		atomic.StoreUint32(&failReal1, 0)
	case <-time.After(10 * time.Second):
		c.Log("fails to wait dp state, step2")
		c.Fail()
		return
	}

	// step 4: making backend is healthy again.
	select {
	case state, ok := <-dpStateChan:
		c.Assert(ok, IsTrue)
		balancers := state.GetBalancers()
		c.Assert(len(balancers), Equals, 1)

		upstreams := balancers[0].GetUpstreams()
		c.Assert(len(upstreams), Equals, 2)

		c.Assert(int(upstreams[0].GetWeight()), Equals, 1000)
		c.Assert(int(upstreams[1].GetWeight()), Equals, 1000)
		c.Assert(len(state.GetDynamicRoutes()), Equals, 1)

		c.Log("failing both backends")
		atomic.StoreUint32(&failReal1, 1)
		atomic.StoreUint32(&failReal2, 1)
	case <-time.After(10 * time.Second):
		c.Log("fails to wait dp state, step1")
		c.Fail()
		return
	}

	// step 5: checking fallback mode.
	select {
	case state, ok := <-dpStateChan:
		c.Assert(ok, IsTrue)
		balancers := state.GetBalancers()
		c.Assert(len(balancers), Equals, 1)

		upstreams := balancers[0].GetUpstreams()
		c.Assert(len(upstreams), Equals, 2)

		// all weights is up in failsafe mode.
		c.Assert(int(upstreams[0].GetWeight()), Equals, 1000)
		c.Assert(int(upstreams[1].GetWeight()), Equals, 1000)
		c.Assert(len(state.GetDynamicRoutes()), Equals, 1)

		c.Log("failing both backends")
		atomic.StoreUint32(&failReal1, 1)
		atomic.StoreUint32(&failReal2, 1)
	case <-time.After(10 * time.Second):
		c.Log("fails to wait dp state, step1")
		c.Fail()
		return
	}

	// no bgp route for upstream without hosts.
	testConfig = &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-1",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    15678,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  3,
					FallCount:  1,
					IntervalMs: 10,
					Checker: &hc_pb.HealthCheckerAttributes{
						Attributes: &hc_pb.HealthCheckerAttributes_Http{
							Http: &hc_pb.HttpCheckerAttributes{
								Scheme: "http",
								Uri:    "/",
								Codes: []uint32{
									http.StatusOK,
								},
							},
						},
					},
				},
				EnableFwmarks: false,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 15678,
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{},
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
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}
	configLoader.configChan <- testConfig
	select {
	case state, ok := <-dpStateChan:
		c.Assert(ok, IsTrue)
		balancers := state.GetBalancers()
		c.Assert(len(balancers), Equals, 1)

		upstreams := balancers[0].GetUpstreams()
		c.Assert(len(upstreams), Equals, 0)

		c.Assert(len(state.GetDynamicRoutes()), Equals, 0)
	case <-time.After(10 * time.Second):
		c.Log("fails to wait dp state")
		c.Fail()
		return
	}
}

// validate logic of generateRoutes call.
func (s *ServicerSuite) TestRouteGenerator(c *C) {
	// servicer.
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	servicer, err := newControlPlaneServicer(
		ctx,
		s.modules,
		time.Millisecond)
	c.Assert(err, NoErr)

	// 1. both lists are empty.
	result, err := servicer.generateRoutes(
		[]*pb.BgpRouteAttributes{}, []*pb.BgpRouteAttributes{})
	c.Assert(err, NoErr)
	c.Assert(result, IsNil)

	// 2. empty list of prohibited routes.
	result, err = servicer.generateRoutes(
		[]*pb.BgpRouteAttributes{
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				Prefixlen: 32,
			},
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
				Prefixlen: 32,
			},
		}, []*pb.BgpRouteAttributes{})
	c.Assert(err, NoErr)
	c.Assert(result, DeepEqualsPretty, []*pb.DynamicRoute{
		{
			Attributes: &pb.DynamicRoute_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix:    &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
					Prefixlen: 32,
				},
			},
		},
		{
			Attributes: &pb.DynamicRoute_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix:    &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
					Prefixlen: 32,
				},
			},
		},
	})

	// 3. one prohibited prefix matches allowed.
	result, err = servicer.generateRoutes(
		[]*pb.BgpRouteAttributes{
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				Prefixlen: 32,
			},
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
				Prefixlen: 32,
			},
		}, []*pb.BgpRouteAttributes{
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
				Prefixlen: 32,
			},
		})
	c.Assert(err, NoErr)
	c.Assert(result, DeepEqualsPretty, []*pb.DynamicRoute{
		{
			Attributes: &pb.DynamicRoute_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix:    &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
					Prefixlen: 32,
				},
			},
		},
	})

	// 3. one prohibited prefix with wider subnet matches both allowed.
	result, err = servicer.generateRoutes(
		[]*pb.BgpRouteAttributes{
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				Prefixlen: 32,
			},
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
				Prefixlen: 32,
			},
		}, []*pb.BgpRouteAttributes{
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.0"}},
				Prefixlen: 24,
			},
		})
	c.Assert(err, NoErr)
	c.Assert(result, IsNil)

	// 4. dups should be excluded.
	result, err = servicer.generateRoutes(
		[]*pb.BgpRouteAttributes{
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				Prefixlen: 32,
			},
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				Prefixlen: 32,
			},
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
				Prefixlen: 32,
			},
		}, []*pb.BgpRouteAttributes{})
	c.Assert(err, NoErr)
	c.Assert(result, DeepEqualsPretty, []*pb.DynamicRoute{
		{
			Attributes: &pb.DynamicRoute_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix:    &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
					Prefixlen: 32,
				},
			},
		},
		{
			Attributes: &pb.DynamicRoute_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix:    &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
					Prefixlen: 32,
				},
			},
		},
	})

	result, err = servicer.generateRoutes(
		[]*pb.BgpRouteAttributes{
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				Prefixlen: 32,
			},
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				Prefixlen: 32,
			},
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
				Prefixlen: 32,
			},
		}, []*pb.BgpRouteAttributes{
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
				Prefixlen: 32,
			},
		})
	c.Assert(err, NoErr)
	c.Assert(result, DeepEqualsPretty, []*pb.DynamicRoute{
		{
			Attributes: &pb.DynamicRoute_BgpAttributes{
				BgpAttributes: &pb.BgpRouteAttributes{
					LocalAsn:  1000,
					PeerAsn:   1000,
					Community: "10000:10000",
					Prefix:    &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
					Prefixlen: 32,
				},
			},
		},
	})

	result, err = servicer.generateRoutes(
		[]*pb.BgpRouteAttributes{
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				Prefixlen: 32,
			},
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				Prefixlen: 32,
			},
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
				Prefixlen: 32,
			},
		}, []*pb.BgpRouteAttributes{
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				Prefixlen: 32,
			},
			{
				LocalAsn:  1000,
				PeerAsn:   1000,
				Community: "10000:10000",
				Prefix: &pb.IP{
					Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
				Prefixlen: 32,
			},
		})
	c.Assert(err, NoErr)
	c.Assert(result, IsNil)
}

// Configuring 3 balancers, two of them announce the same prefix, at the same
// time one of that two has alive_ratio equals 0 which should block prefix
// announcement for both balancers.
func (s *ServicerSuite) TestRouteAnnouncing(c *C) {
	dnsData := make(map[string]*pb.IP)
	dnsData["test-host-1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}}
	dnsData["test-host-2"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.2"}}
	s.modules.DnsResolver = dns_resolver.NewDnsResolverMock(dnsData)

	testConfig := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-http",
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
					IntervalMs: 1,
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
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
			{
				Name:      "test-balancer-https",
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
					IntervalMs: 1,
					Checker:    dummyChecker,
				},
				EnableFwmarks: false,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 443,
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"test-host-1"},
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
				Name:      "test-balancer-third",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "169.0.0.1"}},
									Port:    8080,
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
					IntervalMs: 1,
					Checker:    dummyChecker,
				},
				EnableFwmarks: false,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 8080,
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"test-host-2"},
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
								Address: &pb.IP_Ipv4{Ipv4: "169.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}

	clientChan := make(chan *pb.DataPlaneState, 1)
	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			clientChan <- state
			return nil
		},
	}
	s.modules.DataPlaneClient = client

	content := proto.MarshalTextString(testConfig)
	tmpfile, err := ioutil.TempFile(s.tmpDir, "")
	c.Assert(err, NoErr)
	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, NoErr)

	configLoader := newMockConfigLoader()
	configLoader.configChan <- testConfig
	s.modules.ConfigLoader = configLoader

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	servicer, err := newControlPlaneServicer(
		ctx,
		s.modules,
		time.Millisecond)
	c.Assert(err, NoErr)

	servicer.updateConfig(testConfig)

	// waiting until all balancer will be on.
	for i := 0; i < 10; i++ {
		state, err := servicer.GenerateDataPlaneState()
		c.Assert(err, IsNil)
		if len(state.GetDynamicRoutes()) == 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	servicer.mu.Lock()
	c.Log("balancers:", servicer.balancers)
	balancerHttp := servicer.balancers["test-balancer-http-172.0.0.1:80-tcp"]
	balancerHttp.mutex.Lock()
	balancerHttp.state.Store(&BalancerState{
		States: []*pb.BalancerState{
			{
				Name: "test-balancer-http",
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
				Upstreams: []*pb.UpstreamState{
					{
						Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}},
						Hostname: "test-host-1",
						Port:     80,
						Weight:   1000,
					},
				},
			},
		},
		AliveRatio:   1,
		InitialState: false,
	})
	balancerHttp.mutex.Unlock()

	balancerHttps := servicer.balancers["test-balancer-https-172.0.0.1:443-tcp"]
	balancerHttps.mutex.Lock()
	balancerHttps.state.Store(&BalancerState{
		States: []*pb.BalancerState{
			{
				Name: "test-balancer-https",
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
				Upstreams: []*pb.UpstreamState{
					{
						Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}},
						Hostname: "test-host-1",
						Port:     443,
						Weight:   1000,
					},
				},
			},
		},
		AliveRatio:   1,
		InitialState: false,
	})
	balancerHttps.mutex.Unlock()

	balancerThird := servicer.balancers["test-balancer-third-169.0.0.1:8080-tcp"]
	balancerThird.mutex.Lock()
	balancerThird.state.Store(&BalancerState{
		States: []*pb.BalancerState{
			{
				Name: "test-balancer-third",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "169.0.0.1"}},
									Port:    8080,
								},
							},
						},
					},
				},
				Upstreams: []*pb.UpstreamState{
					{
						Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.2"}},
						Hostname: "test-host-2",
						Port:     8080,
						Weight:   1000,
					},
				},
			},
		},
		AliveRatio:   1,
		InitialState: false,
	})
	balancerThird.mutex.Unlock()
	servicer.mu.Unlock()

	state, err := servicer.GenerateDataPlaneState()
	c.Assert(err, IsNil)

	// dynamic routes should not be there even AliveRatio is positive since
	// balancer still in init state.
	c.Assert(len(state.GetDynamicRoutes()), Equals, 2)

	// fail http balancer via setting empty list of balancers (which equals to
	// undiscovered state).
	balancerHttp.mutex.Lock()
	balancerHttp.state.Store(&BalancerState{
		States:       []*pb.BalancerState{},
		AliveRatio:   0,
		InitialState: true,
	})
	balancerHttp.mutex.Unlock()
	state, err = servicer.GenerateDataPlaneState()
	c.Assert(err, IsNil)
	// dynamic routes should not be since one of balancer is not initialized yet.
	c.Assert(len(state.GetDynamicRoutes()), Equals, 1)

	// fail http balancer.
	servicer.mu.Lock()
	balancerHttp.mutex.Lock()
	balancerHttp.state.Store(&BalancerState{
		States: []*pb.BalancerState{
			{
				Name: "test-balancer-http",
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
				Upstreams: []*pb.UpstreamState{
					{
						Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}},
						Hostname: "test-host-1",
						Port:     80,
						Weight:   0,
					},
				},
			},
		},
		AliveRatio:   0,
		InitialState: false,
	})
	balancerHttp.mutex.Unlock()
	servicer.mu.Unlock()

	state, err = servicer.GenerateDataPlaneState()
	c.Assert(err, IsNil)
	// dynamic routes should not be there even AliveRatio is positive since
	// balancer still in init state.
	c.Assert(len(state.GetDynamicRoutes()), Equals, 1)
}

func (s *ServicerSuite) TestWithdrawTimeout(c *C) {
	dnsData := make(map[string]*pb.IP)
	dnsData["test-host-1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}}
	dnsResolver := dns_resolver.NewDnsResolverMock(dnsData)

	testConfig := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-1",
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
					IntervalMs: 1,
					Checker:    dummyChecker,
				},
				EnableFwmarks: false,
				DynamicRouting: &pb.DynamicRouting{
					AnnounceLimitRatio: 0.9,
					Attributes: &pb.DynamicRouting_BgpAttributes{
						BgpAttributes: &pb.BgpRouteAttributes{
							LocalAsn:  1000,
							PeerAsn:   1000,
							Community: "10000:10000",
							Prefix: &pb.IP{
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen:  32,
							HoldTimeMs: 5000,
						},
					},
				},
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 80,
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"test-host-1"},
						},
					},
				},
			},
		},
	}

	// channel to catch dp state.
	dpStateChan := make(chan *pb.DataPlaneState, 10)
	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			dpStateChan <- state
			return nil
		},
	}

	checkerFactory := NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{})

	discoveryFactory := NewDiscoveryFactory()

	c.Assert(common.ValidateControlPlaneConfig(testConfig), NoErr)

	content := proto.MarshalTextString(testConfig)
	tmpfile, err := ioutil.TempFile(s.tmpDir, "")
	c.Assert(err, NoErr)
	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, NoErr)

	configLoader := newMockConfigLoader()
	configLoader.configChan <- testConfig

	servicerModules := ServicerModules{
		DnsResolver:      dnsResolver,
		CheckerFactory:   checkerFactory,
		DiscoveryFactory: discoveryFactory,
		DataPlaneClient:  client,
		ConfigLoader:     configLoader,
		// No op metric manager.
		FwmarkManager: fwmark.NewManager(5000, 10000),
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	servicer, err := NewControlPlaneServicer(
		ctx,
		servicerModules,
		time.Millisecond)
	c.Assert(err, NoErr)
	c.Assert(servicer, NotNil)

	for i := 0; i < 5; i++ {
		select {
		case state, ok := <-dpStateChan:
			c.Assert(ok, IsTrue)
			if len(state.DynamicRoutes) != 1 {
				continue
			}
			route := state.DynamicRoutes[0]
			switch route.Attributes.(type) {
			case *pb.DynamicRoute_BgpAttributes:
				c.Assert(
					int(route.GetBgpAttributes().GetHoldTimeMs()), Equals, 5000)
			default:
				c.Log("Unknown routing attribute")
				c.Fail()
			}
		case <-time.After(1 * time.Second):
			c.Log("fails to wait closing balancer and its health manager.")
			c.Fail()
		}
	}
}

// Testing control plane behavior with config which contains two balancers
// with the same name, but different vips.
func (s *ServicerSuite) TestDupNamesWithDifferentVips(c *C) {
	tmpDir := c.MkDir()

	dnsData := make(map[string]*pb.IP)
	dnsData["test-host-1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}}
	dnsData["test-host-2"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.2"}}
	dnsResolver := dns_resolver.NewDnsResolverMock(dnsData)

	testConfig := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-1",
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
					IntervalMs: 1,
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
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
			{
				Name:      "test-balancer-1",
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
					IntervalMs: 1,
					Checker:    dummyChecker,
				},
				EnableFwmarks: false,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 443,
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
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}

	expectedDpState := &pb.DataPlaneState{
		Balancers: []*pb.BalancerState{
			{
				Name: "test-balancer-1",
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
				Upstreams: []*pb.UpstreamState{
					{
						Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}},
						Hostname: "test-host-1",
						Port:     80,
						Weight:   1000,
					},
				},
			},
			{
				Name: "test-balancer-1",
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
				Upstreams: []*pb.UpstreamState{
					{
						Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}},
						Hostname: "test-host-1",
						Port:     443,
						Weight:   1000,
					},
				},
			},
		},
		DynamicRoutes: []*pb.DynamicRoute{
			{
				Attributes: &pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &pb.BgpRouteAttributes{
						LocalAsn:  1000,
						PeerAsn:   1000,
						Community: "10000:10000",
						Prefix:    &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
						Prefixlen: 32,
					},
				},
			},
		},
		LinkAddresses: []*pb.LinkAddress{
			{
				LinkName: "lo",
				Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
			},
		},
	}

	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			return nil
		},
	}

	checkerFactory := NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{})

	discoveryFactory := NewDiscoveryFactory()

	c.Assert(common.ValidateControlPlaneConfig(testConfig), NoErr)

	content := proto.MarshalTextString(testConfig)
	tmpfile, err := ioutil.TempFile(tmpDir, "")
	c.Assert(err, NoErr)
	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, NoErr)

	configLoader := newMockConfigLoader()
	configLoader.configChan <- testConfig

	initChan := make(chan struct{})
	servicerModules := ServicerModules{
		DnsResolver:      dnsResolver,
		CheckerFactory:   checkerFactory,
		DiscoveryFactory: discoveryFactory,
		DataPlaneClient:  client,
		ConfigLoader:     configLoader,
		// No op metric manager.
		AfterInitHandler: func() {
			close(initChan)
		},
		FwmarkManager: fwmark.NewManager(5000, 10000),
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	servicer, err := NewControlPlaneServicer(
		ctx,
		servicerModules,
		time.Millisecond)
	c.Assert(err, NoErr)

	// waiting cp initialization.
	select {
	case <-initChan:
	case <-time.After(time.Second):
		c.Log("initialization took too long time.")
		c.Fail()
	}

	state, err := servicer.GetConfiguration(context.Background(), &types.Empty{})
	c.Assert(err, NoErr)
	c.Assert(len(state.GetBalancers()), Equals, 2)
	// make right order to compare whole state since it might be different because
	// of using map inside servicer.
	_, vport, err := common.GetVipFromLbService(state.Balancers[0].GetLbService())
	c.Assert(err, IsNil)
	// make copy to make it possible to modify state without affecting servicer,
	// which might read state.
	state = proto.Clone(state).(*pb.DataPlaneState)
	if vport == 443 {
		tmpBalancer := state.Balancers[0]
		state.Balancers[0] = state.Balancers[1]
		state.Balancers[1] = tmpBalancer
	}
	c.Assert(state, DeepEqualsPretty, expectedDpState)
}

// Validate generation of ip rules and link addresses.
func (s *ServicerSuite) TestRulesAndLinkAddresses(c *C) {
	// 1. create control_plane config.
	testConfig := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-1",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
									Port:    15678,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  3,
					FallCount:  1,
					IntervalMs: 10,
					Checker:    dummyChecker,
				},
				EnableFwmarks: true,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 15678,
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"127.1.0.1", "127.1.0.2"},
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
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
			{
				Name:      "test-balancer-2",
				SetupName: "setup1",
				LbService: &pb.LoadBalancerService{
					Service: &pb.LoadBalancerService_IpvsService{
						IpvsService: &pb.IpvsService{
							Attributes: &pb.IpvsService_TcpAttributes{
								TcpAttributes: &pb.IpvsTcpAttributes{
									Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
									Port:    15678,
								},
							},
						},
					},
				},
				UpstreamRouting: &pb.UpstreamRouting{
					ForwardMethod: pb.ForwardMethods_TUNNEL,
				},
				UpstreamChecker: &hc_pb.UpstreamChecker{
					RiseCount:  3,
					FallCount:  1,
					IntervalMs: 10,
					Checker:    dummyChecker,
				},
				EnableFwmarks: true,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 15678,
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{"127.1.0.1"},
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
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}

	// channel to catch dp state.
	dpStateChan := make(chan *pb.DataPlaneState, 10)
	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			dpStateChan <- state
			return nil
		},
	}

	dnsData := make(map[string]*pb.IP)
	dnsData["127.1.0.1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "127.1.0.1"}}
	dnsData["127.1.0.2"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "127.1.0.2"}}
	dnsData["127.1.0.3"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "127.1.0.3"}}
	dnsData["127.2.0.1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "127.2.0.1"}}
	dnsResolver := dns_resolver.NewDnsResolverMock(dnsData)

	checkerFactory := NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{
		SourceIPv4: net.IP{},
	})

	discoveryFactory := NewDiscoveryFactory()

	c.Assert(common.ValidateControlPlaneConfig(testConfig), NoErr)

	content := proto.MarshalTextString(testConfig)
	tmpfile, err := ioutil.TempFile(s.tmpDir, "")
	c.Assert(err, NoErr)
	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, NoErr)

	configLoader := newMockConfigLoader()
	configLoader.configChan <- testConfig

	servicerModules := ServicerModules{
		DnsResolver:      dnsResolver,
		CheckerFactory:   checkerFactory,
		DiscoveryFactory: discoveryFactory,
		DataPlaneClient:  client,
		ConfigLoader:     configLoader,
		FwmarkManager:    fwmark.NewManager(5000, 10000),
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	_, err = NewControlPlaneServicer(
		ctx,
		servicerModules,
		time.Minute)
	c.Assert(err, NoErr)
	for i := 0; i < 3; i++ {
		select {
		case state, ok := <-dpStateChan:
			c.Assert(ok, IsTrue)
			// waiting healthy state of all backends.
			balancers := state.GetBalancers()
			if i != len(testConfig.Balancers) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			c.Assert(len(balancers), Equals, 4) // two configured + 2 fwmark generated.
			c.Assert(len(state.GetLinkAddresses()), Equals, 2)
			sort.Slice(state.LinkAddresses, func(i, j int) bool {
				return state.LinkAddresses[i].String() <
					state.LinkAddresses[j].String()
			})
			c.Assert(state.LinkAddresses, DeepEqualsPretty, []*pb.LinkAddress{
				{
					LinkName: "lo",
					Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
				},
				{
					LinkName: "lo",
					Address:  &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.2"}},
				},
			})
		case <-time.After(10 * time.Second):
			c.Log("fails to wait dp state, step1")
			c.Fail()
			return
		}
	}
}

// Validate that callback is called only after initialized balancers.
func (s *ServicerSuite) TestAfterInitCallback(c *C) {
	// TODO(dkopytkov): simplify setting up part - required too much coding today.
	// 1. Preparation...
	dnsData := make(map[string]*pb.IP)
	dnsData["test-host-1"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.1"}}
	dnsData["test-host-2"] = &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "1.1.2.2"}}
	dnsResolver := dns_resolver.NewDnsResolverMock(dnsData)

	client := mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			return nil
		},
	}

	checkerFactory := NewHealthCheckerFactory(BaseHealthCheckerFactoryParams{})

	discoveryFactory := NewDiscoveryFactory()

	testConfig := &pb.ControlPlaneConfig{
		Balancers: []*pb.BalancerConfig{
			{
				Name:      "test-balancer-1",
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
					IntervalMs: 1,
					Checker:    dummyChecker,
				},
				EnableFwmarks: false,
				UpstreamDiscovery: &pb.UpstreamDiscovery{
					Port: 80,
					Attributes: &pb.UpstreamDiscovery_StaticAttributes{
						StaticAttributes: &pb.StaticDiscoveryAttributes{
							Hosts: []string{},
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
								Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Prefixlen: 32,
						},
					},
				},
			},
		},
	}

	content := proto.MarshalTextString(testConfig)
	tmpfile, err := ioutil.TempFile(s.tmpDir, "")
	c.Assert(err, NoErr)
	_, err = tmpfile.Write([]byte(content))
	c.Assert(err, NoErr)

	configLoader := newMockConfigLoader()
	configLoader.configChan <- testConfig

	servicerModules := ServicerModules{
		DnsResolver:      dnsResolver,
		CheckerFactory:   checkerFactory,
		DiscoveryFactory: discoveryFactory,
		DataPlaneClient:  client,
		ConfigLoader:     configLoader,
		// No op metric manager.
		FwmarkManager: fwmark.NewManager(5000, 10000),
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	servicer, err := newControlPlaneServicer(
		ctx,
		servicerModules,
		time.Millisecond)
	c.Assert(err, NoErr)
	c.Assert(servicer.initialState, IsTrue)

	// Test 1 - callback should not be called since no any balancers yet.
	servicer.modules.AfterInitHandler = func() {
		c.Log("unexpected call of AfterInitHandler()")
		c.Fail()
	}
	err = servicer.applyDataPlaneState(&pb.DataPlaneState{})
	c.Assert(err, IsNil)

	// Test 2 - callback should not be called because no generates balancer
	// states yet.
	servicer.updateConfig(testConfig)
	servicer.balancers["test-balancer-1-172.0.0.1:80-tcp"].state.Store(&BalancerState{
		States:       []*pb.BalancerState{},
		AliveRatio:   0,
		InitialState: true,
	})
	_, err = servicer.GenerateDataPlaneState()
	c.Assert(err, IsNil)
	c.Assert(servicer.initialState, IsTrue)

	err = servicer.applyDataPlaneState(&pb.DataPlaneState{})
	c.Assert(err, IsNil)

	// Test 3 - callback is called because balancer provides its states.
	servicer.balancers["test-balancer-1-172.0.0.1:80-tcp"].state.Store(&BalancerState{
		States: []*pb.BalancerState{
			{
				Name: "test-balancer-1",
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
				Upstreams: []*pb.UpstreamState{},
			},
		},
		AliveRatio:   0,
		InitialState: true,
	})

	_, err = servicer.GenerateDataPlaneState()
	c.Assert(err, IsNil)
	c.Assert(servicer.initialState, IsFalse)

	// Test 3.1 - no called callback until first successful applied state.
	servicer.modules.DataPlaneClient = mockDpClient{
		setFunc: func(state *pb.DataPlaneState) error {
			return fmt.Errorf("fails to send")
		},
	}
	err = servicer.applyDataPlaneState(&pb.DataPlaneState{})
	c.Assert(err, NotNil)

	// Test 3.2 - successful callback call.
	servicer.modules.DataPlaneClient = client
	notifyChan := make(chan error)
	servicer.modules.AfterInitHandler = func() {
		close(notifyChan)
	}
	err = servicer.applyDataPlaneState(&pb.DataPlaneState{})
	c.Assert(err, IsNil)

	select {
	case <-notifyChan:
	case <-time.After(5 * time.Second):
		c.Log("callback is not called.")
		c.Fail()
	}

	// Test 4 - no more calls of callback (double close of notifyChan will panic).
	c.Assert(servicer.modules.AfterInitHandler, IsNil)
	err = servicer.applyDataPlaneState(&pb.DataPlaneState{})
	c.Assert(err, IsNil)
}
