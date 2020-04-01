package common

import (
	"fmt"
	"sort"
	"strings"

	kglb_pb "dropbox/proto/kglb"
	"godropbox/errors"
)

// Prettify DataPlaneState.
func PrettyDataPlaneState(state *kglb_pb.DataPlaneState) (string, error) {

	var out string

	out = out + "----- Balancers\n"
	out = out + `Name Prot LocalAddress:Port Scheduler Flags
  -> RemoteAddress:Port Forward Weight` + "\n"

	for _, balancer := range state.GetBalancers() {
		// pretty lb.
		lb, err := PrettyLoadBalancerService(balancer.GetLbService())
		if err != nil {
			return "", err
		}
		out = out + balancer.GetName() + " " + lb + "\n"

		// pretty upstreams after sorting.
		upstreams := balancer.GetUpstreams()
		// sorting by address.
		sort.Slice(upstreams, func(i, j int) bool {
			return upstreams[i].GetAddress().String() <
				upstreams[j].GetAddress().String()
		})
		for _, up := range upstreams {
			pretty, err := PrettyUpstreamState(up)
			if err != nil {
				return "", err
			}
			out = out + fmt.Sprintf("  -> %s\n", pretty)
		}
	}

	// dump set of DynamicRoute.
	out = out + "----- Dynamic Route Map\n"
	out = out + "LocalAsn PeerAsn Community Prefix/Prefixlen\n"
	for _, routing := range state.GetDynamicRoutes() {
		pretty, err := PrettyDynamicRoute(routing)
		if err != nil {
			return "", err
		}
		out = out + " - " + pretty + "\n"
	}

	// dump set of LinkAddress.
	out = out + "----- Address Map\n"
	for _, link := range state.GetLinkAddresses() {
		pretty, err := PrettyLinkAddress(link)
		if err != nil {
			return "", err
		}
		out = out + " - " + pretty + "\n"
	}

	return out, nil
}

// Pretty LinkAddress struct.
func PrettyDynamicRoute(routing *kglb_pb.DynamicRoute) (string, error) {
	// extract bgp.
	switch routing.Attributes.(type) {
	case *kglb_pb.DynamicRoute_BgpAttributes:
		bgp := routing.GetBgpAttributes()
		return fmt.Sprintf(
			"%d %d %s %s/%d",
			bgp.GetLocalAsn(),
			bgp.GetPeerAsn(),
			// replacing space separator in community  to improve eye view of
			// the string since space is used in arg separation as well.
			strings.Replace(bgp.GetCommunity(), " ", ",", -1),
			KglbAddrToNetIp(bgp.GetPrefix()),
			bgp.GetPrefixlen()), nil
	default:
		return "", errors.Newf("Unknown routing attribute: %+v", routing)
	}
}

// Pretty LinkAddress struct.
func PrettyLinkAddress(link *kglb_pb.LinkAddress) (string, error) {
	return fmt.Sprintf(
		"%s%%%s",
		KglbAddrToNetIp(link.GetAddress()),
		link.GetLinkName()), nil
}

// Pretty UpstreamState
func PrettyUpstreamState(up *kglb_pb.UpstreamState) (string, error) {
	fw, err := PrettyForwardMethod(up.GetForwardMethod())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"%s|%s:%d %s %d",
		up.GetHostname(),
		KglbAddrToNetIp(up.GetAddress()),
		up.GetPort(),
		fw,
		up.GetWeight()), nil
}

// Pretty LoadBalancerService
func PrettyLoadBalancerService(lb *kglb_pb.LoadBalancerService) (string, error) {

	var out string

	ipvsService, err := GetIpvsServiceFromLbService(lb)
	if err != nil {
		return "", err
	}

	// 1. pretty service.
	switch attr := ipvsService.Attributes.(type) {
	case *kglb_pb.IpvsService_TcpAttributes:
		out += fmt.Sprintf(
			"TCP  %s:%d",
			KglbAddrToNetIp(attr.TcpAttributes.GetAddress()),
			uint16(attr.TcpAttributes.Port))
	case *kglb_pb.IpvsService_UdpAttributes:
		out += fmt.Sprintf(
			"UDP  %s:%d",
			KglbAddrToNetIp(attr.UdpAttributes.GetAddress()),
			uint16(attr.UdpAttributes.Port))
	case *kglb_pb.IpvsService_FwmarkAttributes:
		out += fmt.Sprintf("FWM  %d", attr.FwmarkAttributes.Fwmark)
	default:
		return "", errors.Newf("Unknown attributes type: %s", attr)
	}

	// 2. pretty scheduler
	scheduler, err := PrettyIpvsScheduler(ipvsService.Scheduler)
	if err != nil {
		return "", err
	}
	out = out + " " + scheduler

	return out, nil
}

// Pretty ForwardMethod.
func PrettyForwardMethod(fw kglb_pb.ForwardMethods) (string, error) {
	switch fw {
	case kglb_pb.ForwardMethods_TUNNEL:
		return "Tunnel", nil
	case kglb_pb.ForwardMethods_MASQ:
		return "Masq", nil
	default:
		return "", errors.Newf("unknown ForwardMethod: %v", fw)
	}
}

// Pretty IpvsSchceduler.
func PrettyIpvsScheduler(scheduler kglb_pb.IpvsService_Scheduler) (string, error) {
	switch scheduler {
	case kglb_pb.IpvsService_RR:
		return "rr", nil
	case kglb_pb.IpvsService_WRR:
		return "wrr", nil
	case kglb_pb.IpvsService_IP_VS_SCH:
		return "ip_vs_sch", nil
	default:
		return "", errors.Newf("unknown kglb ipvs scheduler: %v", scheduler)
	}
}
