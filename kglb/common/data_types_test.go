package common

import (
	. "gopkg.in/check.v1"

	kglb_pb "dropbox/proto/kglb"
)

type DataTypesSuite struct{}

var _ = Suite(&DataTypesSuite{})

func (s *DataTypesSuite) TestKglbAddrToAddressFamily(c *C) {
	c.Assert(KglbAddrToAddressFamily(&kglb_pb.IP{
		Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}}), Equals, "v4")
	c.Assert(KglbAddrToAddressFamily(
		&kglb_pb.IP{Address: &kglb_pb.IP_Ipv6{Ipv6: "::1"}}), Equals, "v6")
}

func (s *DataTypesSuite) TestGetKeyFromLbService(c *C) {
	key, err :=
		GetKeyFromLbService(&kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
			IpvsService: &kglb_pb.IpvsService{
				Attributes: &kglb_pb.IpvsService_TcpAttributes{
					TcpAttributes: &kglb_pb.IpvsTcpAttributes{
						Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
						Port:    443,
					}},
				Scheduler: kglb_pb.IpvsService_RR,
			}}})
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "172.0.0.1:443-tcp")

	key, err =
		GetKeyFromLbService(&kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
			IpvsService: &kglb_pb.IpvsService{
				Attributes: &kglb_pb.IpvsService_TcpAttributes{
					TcpAttributes: &kglb_pb.IpvsTcpAttributes{
						Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv6{Ipv6: "::1"}},
						Port:    443,
					}},
				Scheduler: kglb_pb.IpvsService_RR,
			}}})
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "::1:443-tcp")

	key, err =
		GetKeyFromLbService(&kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
			IpvsService: &kglb_pb.IpvsService{
				Attributes: &kglb_pb.IpvsService_UdpAttributes{
					UdpAttributes: &kglb_pb.IpvsUdpAttributes{
						Address: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{
								Ipv4: "127.0.0.1",
							},
						},
						Port: 80,
					}},
				Scheduler: kglb_pb.IpvsService_RR,
			}}})
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "127.0.0.1:80-udp")

	key, err =
		GetKeyFromLbService(&kglb_pb.LoadBalancerService{Service: &kglb_pb.LoadBalancerService_IpvsService{
			IpvsService: &kglb_pb.IpvsService{
				Attributes: &kglb_pb.IpvsService_FwmarkAttributes{
					FwmarkAttributes: &kglb_pb.IpvsFwmarkAttributes{
						AddressFamily: kglb_pb.AddressFamily_AF_INET,
						Fwmark:        1010,
					}},
				Scheduler: kglb_pb.IpvsService_RR,
			}}})
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "fwmark:1010:v4")
}
