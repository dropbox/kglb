package common

import (
	. "gopkg.in/check.v1"

	kglb_pb "dropbox/proto/kglb"
)

type PrettySuite struct {
}

var _ = Suite(&PrettySuite{})

func (m *PrettySuite) TestPrettyScheduler(c *C) {
	pretty, err := PrettyIpvsScheduler(kglb_pb.IpvsService_RR)
	c.Assert(err, IsNil)
	c.Assert(pretty, Equals, "rr")
}

func (m *PrettySuite) TestPrettyForwardMethod(c *C) {
	pretty, err := PrettyForwardMethod(kglb_pb.ForwardMethods_TUNNEL)
	c.Assert(err, IsNil)
	c.Assert(pretty, Equals, "Tunnel")
}

func (m *PrettySuite) TestPrettyUpstreamState(c *C) {
	pretty, err := PrettyUpstreamState(&kglb_pb.UpstreamState{
		Address:       &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.1"}},
		Port:          443,
		Hostname:      "hostname1",
		Weight:        50,
		ForwardMethod: kglb_pb.ForwardMethods_TUNNEL,
	})
	c.Assert(err, IsNil)
	c.Assert(pretty, Equals, "hostname1|10.0.0.1:443 Tunnel 50")
}

func (m *PrettySuite) TestPrettyLoadBalancerService(c *C) {
	pretty, err := PrettyLoadBalancerService(
		&kglb_pb.LoadBalancerService{
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
		})
	c.Assert(err, IsNil)
	c.Assert(pretty, Equals, "TCP  172.0.0.1:443 rr")
}

func (m *PrettySuite) TestPrettyLinkAddress(c *C) {
	pretty, err := PrettyLinkAddress(&kglb_pb.LinkAddress{
		LinkName: "lo",
		Address:  &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
	})
	c.Assert(err, IsNil)
	c.Assert(pretty, Equals, "172.0.0.1%lo")
}

func (m *PrettySuite) TestPrettyDynamicRoute(c *C) {
	pretty, err := PrettyDynamicRoute(&kglb_pb.DynamicRoute{
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
	})
	c.Assert(err, IsNil)
	c.Assert(pretty, Equals, "10 20 my_community 10.0.0.2/32")
}

func (m *PrettySuite) TestPrettyDataPlaneState(c *C) {
	pretty, err := PrettyDataPlaneState(&kglb_pb.DataPlaneState{
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
						Community: "65101:30000 65102:10090 65103:10000",
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
						Community: "65101:30000 65102:10090 65103:10000",
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
	})
	c.Assert(err, IsNil)
	c.Assert(pretty, Equals, `----- Balancers
Name Prot LocalAddress:Port Scheduler Flags
  -> RemoteAddress:Port Forward Weight
TestName1 TCP  172.0.0.1:443 rr
  -> hostname1|10.0.0.1:443 Tunnel 50
  -> hostname2|10.0.0.2:443 Tunnel 50
TestName2 TCP  172.0.0.2:443 rr
  -> hostname2|10.0.0.2:443 Tunnel 50
----- Dynamic Route Map
LocalAsn PeerAsn Community Prefix/Prefixlen
 - 10 20 65101:30000,65102:10090,65103:10000 10.0.0.2/32
 - 30 40 65101:30000,65102:10090,65103:10000 10.0.0.5/32
----- Address Map
 - 172.0.0.1%lo
 - 172.0.0.2%lo
`)
}
