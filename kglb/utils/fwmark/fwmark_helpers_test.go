package fwmark

import (
	"testing"

	"github.com/stretchr/testify/require"

	kglb_pb "dropbox/proto/kglb"
)

var (
	service1 = &kglb_pb.IpvsService{
		Attributes: &kglb_pb.IpvsService_TcpAttributes{
			TcpAttributes: &kglb_pb.IpvsTcpAttributes{
				Address: &kglb_pb.IP{Address: &kglb_pb.IP_Ipv4{Ipv4: "4.3.2.1"}},
				Port:    443,
			},
		},
		Scheduler: kglb_pb.IpvsService_RR,
	}

	service2 = &kglb_pb.IpvsService{
		Attributes: &kglb_pb.IpvsService_FwmarkAttributes{
			FwmarkAttributes: &kglb_pb.IpvsFwmarkAttributes{
				AddressFamily: kglb_pb.AddressFamily_AF_INET,
				Fwmark:        10,
			},
		},
		Scheduler: kglb_pb.IpvsService_RR,
		Flags: []kglb_pb.IpvsService_Flag{
			kglb_pb.IpvsService_ONEPACKET,
		},
	}
)

func TestIsFwmarkService(t *testing.T) {
	require.False(t, IsFwmarkService(service1))
	require.True(t, IsFwmarkService(service2))
}
