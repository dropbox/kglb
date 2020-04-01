package common

import (
	"fmt"
	"net"

	"github.com/gogo/protobuf/proto"
	"godropbox/errors"

	kglb_pb "dropbox/proto/kglb"
)

func NetIpToKglbAddr(netIP net.IP) *kglb_pb.IP {
	res := &kglb_pb.IP{}
	if ip := netIP.To4(); ip != nil {
		res.Address = &kglb_pb.IP_Ipv4{Ipv4: netIP.String()}
	} else {
		res.Address = &kglb_pb.IP_Ipv6{Ipv6: netIP.String()}
	}
	return res
}

func KglbAddrToAddressFamily(kglbIP *kglb_pb.IP) string {
	switch kglbIP.GetAddress().(type) {
	case *kglb_pb.IP_Ipv4:
		return "v4"
	case *kglb_pb.IP_Ipv6:
		return "v6"
	default:
		return ""
	}
}

func KglbAddrToNetIp(kglbIP *kglb_pb.IP) net.IP {
	switch addr := kglbIP.GetAddress().(type) {
	case *kglb_pb.IP_Ipv4:
		return net.ParseIP(addr.Ipv4)
	case *kglb_pb.IP_Ipv6:
		return net.ParseIP(addr.Ipv6)
	default:
		return nil
	}
}

func KglbAddrToString(kglbIP *kglb_pb.IP) string {
	switch addr := kglbIP.GetAddress().(type) {
	case *kglb_pb.IP_Ipv4:
		return net.ParseIP(addr.Ipv4).String()
	case *kglb_pb.IP_Ipv6:
		return fmt.Sprintf("[%s]", net.ParseIP(addr.Ipv6).String())
	default:
		return ""
	}
}

func KglbAddrToNetIpNet(kglbIP *kglb_pb.IP) *net.IPNet {
	switch addr := kglbIP.GetAddress().(type) {
	case *kglb_pb.IP_Ipv4:
		return &net.IPNet{
			IP:   net.ParseIP(addr.Ipv4),
			Mask: net.CIDRMask(32, 32)}
	case *kglb_pb.IP_Ipv6:
		return &net.IPNet{
			IP:   net.ParseIP(addr.Ipv6),
			Mask: net.CIDRMask(128, 128)}
	default:
		return nil
	}
}

func KglbAddrToFamily(kglbIP *kglb_pb.IP) kglb_pb.AddressFamily {
	switch kglbIP.GetAddress().(type) {
	case *kglb_pb.IP_Ipv6:
		return kglb_pb.AddressFamily_AF_INET6
	default:
		return kglb_pb.AddressFamily_AF_INET
	}
}

func UpstreamsEqual(s, o *kglb_pb.UpstreamState) bool {
	s = proto.Clone(s).(*kglb_pb.UpstreamState)
	o = proto.Clone(o).(*kglb_pb.UpstreamState)
	s.Weight = 0
	o.Weight = 0
	s.Hostname = ""
	o.Hostname = ""
	return proto.Equal(s, o)
}

func IPVSServicesEqual(s, o *kglb_pb.IpvsService) bool {
	return proto.Equal(s, o)
}

func GetIpvsServiceFromLbService(
	lbService *kglb_pb.LoadBalancerService) (*kglb_pb.IpvsService, error) {

	var ipvsService *kglb_pb.IpvsService
	switch lbService.Service.(type) {
	case *kglb_pb.LoadBalancerService_IpvsService:
		ipvsService = lbService.GetIpvsService()
	default:
		return nil, errors.Newf("Unknown lb service type: %+v", lbService)
	}

	if ipvsService == nil {
		return nil, errors.Newf("missed ipvs service state: %+v", lbService)
	}
	return ipvsService, nil
}

func GetIpvsServiceFromBalancer(
	balancer *kglb_pb.BalancerState) (*kglb_pb.IpvsService, error) {

	// check arg first.
	if balancer == nil {
		return nil, errors.New("balancer state is empty.")
	}

	return GetIpvsServiceFromLbService(balancer.GetLbService())
}

// Returns vip:port from LoadBalancerService proto.
func GetVipFromLbService(lb *kglb_pb.LoadBalancerService) (string, int, error) {
	// getting IpvsService first.
	ipvsService, err := GetIpvsServiceFromLbService(lb)
	if err != nil {
		return "", 0, err
	}

	// extract vip, port, fwmark
	switch attr := ipvsService.Attributes.(type) {
	case *kglb_pb.IpvsService_TcpAttributes:
		ip, port := KglbAddrToNetIp(attr.TcpAttributes.GetAddress()).String(),
			int(attr.TcpAttributes.Port)
		return ip, port, nil
	case *kglb_pb.IpvsService_UdpAttributes:
		ip, port := KglbAddrToNetIp(attr.UdpAttributes.GetAddress()).String(),
			int(attr.UdpAttributes.Port)
		return ip, port, nil
	case *kglb_pb.IpvsService_FwmarkAttributes:
		return "fwmark",
			int(attr.FwmarkAttributes.GetFwmark()),
			nil
	default:
		return "", 0, fmt.Errorf(
			"unknown attributes type of ipvs service: %+v: ",
			ipvsService)
	}
}

// Generate unique key based on balancer config. It includes name and vip:vport,
// and proto (tcp/udp) today which means control plane will need to recreate
// balancer in case of dynamic update of one of that attribute. It is done in
// this way to simplify logic of updating balancer without updating its
// BalancerStats instance since all these attributes inside tags hierarchy.
func GetKeyFromBalancerConfig(balancerConfig *kglb_pb.BalancerConfig) (string, error) {
	// balancer name.
	balancerName := balancerConfig.GetName()
	// key based on vip:vport and proto
	lbKey, err := GetKeyFromLbService(balancerConfig.GetLbService())
	if err != nil {
		return "", err
	}

	return balancerName + "-" + lbKey, nil
}

// construct key for LoadBalancerService in "vip:vport-proto" format.
func GetKeyFromLbService(lb *kglb_pb.LoadBalancerService) (string, error) {
	// getting IpvsService first.
	ipvsService, err := GetIpvsServiceFromLbService(lb)
	if err != nil {
		return "", err
	}

	// extract vip, port, fwmark
	switch attr := ipvsService.Attributes.(type) {
	case *kglb_pb.IpvsService_TcpAttributes:
		ip, port := KglbAddrToNetIp(attr.TcpAttributes.GetAddress()).String(),
			int(attr.TcpAttributes.Port)
		return fmt.Sprintf("%s:%d-tcp", ip, port), nil
	case *kglb_pb.IpvsService_UdpAttributes:
		ip, port := KglbAddrToNetIp(attr.UdpAttributes.GetAddress()).String(),
			int(attr.UdpAttributes.Port)
		return fmt.Sprintf("%s:%d-udp", ip, port), nil
	case *kglb_pb.IpvsService_FwmarkAttributes:
		af := ""
		if attr.FwmarkAttributes.AddressFamily == kglb_pb.AddressFamily_AF_INET {
			af = "v4"
		} else if attr.FwmarkAttributes.AddressFamily == kglb_pb.AddressFamily_AF_INET6 {
			af = "v6"
		}
		return fmt.Sprintf(
			"fwmark:%d:%s",
			int(attr.FwmarkAttributes.GetFwmark()),
			af), nil
	default:
		return "", fmt.Errorf(
			"unknown attributes type of ipvs service: %+v: ",
			ipvsService)
	}
}
