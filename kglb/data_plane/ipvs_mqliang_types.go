package data_plane

import (
	"net"
	"sort"
	"syscall"

	"github.com/mqliang/libipvs"

	"dropbox/kglb/common"
	kglb_pb "dropbox/proto/kglb"
	"godropbox/errors"
)

var supportedFlags = map[kglb_pb.IpvsService_Flag]uint32{
	kglb_pb.IpvsService_PERSISTENT:  libipvs.IP_VS_SVC_F_PERSISTENT,
	kglb_pb.IpvsService_HASHED:      libipvs.IP_VS_SVC_F_HASHED,
	kglb_pb.IpvsService_ONEPACKET:   libipvs.IP_VS_SVC_F_ONEPACKET,
	kglb_pb.IpvsService_SH_FALLBACK: libipvs.IP_VS_SVC_F_SCHED_SH_FALLBACK,
	kglb_pb.IpvsService_SH_PORT:     libipvs.IP_VS_SVC_F_SCHED_SH_PORT,

	// IP_VS_SVC_F_SCHED* flags are not supported as they are aliases:
	// IP_VS_SVC_F_SCHED_SH_FALLBACK = IP_VS_SVC_F_SCHED1 /* SH fallback */
	// IP_VS_SVC_F_SCHED_SH_PORT     = IP_VS_SVC_F_SCHED2 /* SH use port */

	//kglb_pb.IpvsService_SCHED1:      libipvs.IP_VS_SVC_F_SCHED1,
	//kglb_pb.IpvsService_SCHED2:      libipvs.IP_VS_SVC_F_SCHED2,
	//kglb_pb.IpvsService_SCHED3:      libipvs.IP_VS_SVC_F_SCHED3,
}

func toLibIpvsFlags(kglbFlags []kglb_pb.IpvsService_Flag) (libipvs.Flags, error) {
	flags := libipvs.Flags{
		Mask: ^uint32(0),
	}

	for _, kglbFlag := range kglbFlags {
		libIpvsFlag, ok := supportedFlags[kglbFlag]
		if !ok {
			return flags, errors.Newf("Unsupported kglb flag: %s", kglbFlag)
		}
		// set flag
		flags.Flags |= libIpvsFlag
	}
	return flags, nil
}

func toKglbFlags(ipvsFlags libipvs.Flags) ([]kglb_pb.IpvsService_Flag, error) {
	if ipvsFlags.Mask != ^uint32(0) {
		return nil, errors.Newf("Unsupported mask: %v", ipvsFlags.Mask)
	}
	kglbFlags := make([]kglb_pb.IpvsService_Flag, 0)

	// unset HASHED flag as it's set by ipvs for all services
	// see https://github.com/mqliang/libipvs/blob/7fc48254c184ef71e2f10f36bc8aa120873a1485/ipvs_test.go#L184
	if ipvsFlags.Flags&libipvs.IP_VS_SVC_F_HASHED != 0 {
		ipvsFlags.Flags ^= libipvs.IP_VS_SVC_F_HASHED
	}

	for kglbFlag, libIpvsFlag := range supportedFlags {
		if ipvsFlags.Flags&libIpvsFlag != 0 {
			kglbFlags = append(kglbFlags, kglbFlag)
			//unset flag
			ipvsFlags.Flags ^= libIpvsFlag
		}
	}

	// check if any flags left
	if ipvsFlags.Flags != 0 {
		return nil, errors.Newf("Unsupported flags left: %d", ipvsFlags.Flags)
	}

	sort.Slice(kglbFlags, func(i, j int) bool { return kglbFlags[i] < kglbFlags[j] })
	return kglbFlags, nil
}

func libipvsTokglbStats(stats *libipvs.Stats) *kglb_pb.Stats {
	return &kglb_pb.Stats{
		ConnectionsCount: uint64(stats.Connections),
		PacketsInCount:   uint64(stats.PacketsIn),
		PacketsOutCount:  uint64(stats.PacketsOut),
		BytesInCount:     uint64(stats.BytesIn),
		BytesOutCount:    uint64(stats.BytesOut),
		ConnectionsRate:  uint64(stats.CPS),
		PacketsInRate:    uint64(stats.PPSIn),
		PacketsOutRate:   uint64(stats.PPSOut),
		BytesInRate:      uint64(stats.BPSIn),
		BytesOutRate:     uint64(stats.BPSOut),
	}
}

func tokglbVirtualService(service *libipvs.Service) (*kglb_pb.IpvsService, error) {
	var err error
	kglbIpvsService := &kglb_pb.IpvsService{}

	if kglbIpvsService.Scheduler, err = tokglbIPVSScheduler(service.SchedName); err != nil {
		return nil, err
	}

	if kglbIpvsService.Flags, err = toKglbFlags(service.Flags); err != nil {
		return nil, err
	}

	if service.FWMark != 0 {
		kglbIpvsService.Attributes = &kglb_pb.IpvsService_FwmarkAttributes{
			FwmarkAttributes: &kglb_pb.IpvsFwmarkAttributes{
				AddressFamily: tokglbAddressFamily(service.AddressFamily),
				Fwmark:        service.FWMark,
			},
		}
	} else {
		protocol, err := libipvsProtocolTokglbProtocol(service.Protocol)
		if err != nil {
			return nil, err
		}
		switch protocol {
		case kglb_pb.IPProtocol_TCP:
			kglbIpvsService.Attributes = &kglb_pb.IpvsService_TcpAttributes{TcpAttributes: &kglb_pb.IpvsTcpAttributes{
				Address: common.NetIpToKglbAddr(service.Address),
				Port:    uint32(service.Port),
			}}
		case kglb_pb.IPProtocol_UDP:
			kglbIpvsService.Attributes = &kglb_pb.IpvsService_UdpAttributes{UdpAttributes: &kglb_pb.IpvsUdpAttributes{
				Address: common.NetIpToKglbAddr(service.Address),
				Port:    uint32(service.Port),
			}}
		}
	}
	return kglbIpvsService, nil
}

func tokglbRealServer(destination *libipvs.Destination) (*kglb_pb.UpstreamState, error) {
	fwd, err := tokglbUpstreamForwardMethod(destination.FwdMethod)
	if err != nil {
		return nil, err
	}

	return &kglb_pb.UpstreamState{
		Address:       common.NetIpToKglbAddr(destination.Address),
		Port:          uint32(destination.Port),
		Weight:        destination.Weight,
		ForwardMethod: fwd,
	}, nil
}

func toLibipvsDestination(realServer *kglb_pb.UpstreamState) (*libipvs.Destination, error) {
	ip := common.KglbAddrToNetIp(realServer.Address)
	fwd, err := tolibForwardMethod(realServer.ForwardMethod)
	if err != nil {
		return nil, err
	}

	return &libipvs.Destination{
		Address:       ip,
		Port:          uint16(realServer.GetPort()),
		Weight:        realServer.Weight,
		AddressFamily: ipToLibipvsAddressFamily(ip),
		FwdMethod:     fwd,
	}, nil
}

func toLibipvsService(service *kglb_pb.IpvsService) (*libipvs.Service, error) {
	var err error
	result := &libipvs.Service{}

	if result.SchedName, err = tolibIPVSScheduler(service.Scheduler); err != nil {
		return nil, err
	}

	if result.Flags, err = toLibIpvsFlags(service.Flags); err != nil {
		return nil, err
	}

	switch attr := service.Attributes.(type) {
	case *kglb_pb.IpvsService_TcpAttributes:
		vip := common.KglbAddrToNetIp(attr.TcpAttributes.GetAddress())
		result.Address = vip
		result.Protocol = syscall.IPPROTO_TCP
		result.Port = uint16(attr.TcpAttributes.Port)
		result.AddressFamily = ipToLibipvsAddressFamily(vip)
		result.Netmask = netmaskByAddressFamily(result.AddressFamily)
	case *kglb_pb.IpvsService_UdpAttributes:
		vip := common.KglbAddrToNetIp(attr.UdpAttributes.GetAddress())
		result.Address = vip
		result.Protocol = syscall.IPPROTO_UDP
		result.Port = uint16(attr.UdpAttributes.GetPort())
		result.AddressFamily = ipToLibipvsAddressFamily(vip)
		result.Netmask = netmaskByAddressFamily(result.AddressFamily)
	case *kglb_pb.IpvsService_FwmarkAttributes:
		result.AddressFamily = tolibAddressFamily(attr.FwmarkAttributes.GetAddressFamily())
		result.FWMark = attr.FwmarkAttributes.Fwmark
		result.Netmask = netmaskByAddressFamily(result.AddressFamily)
	default:
		return nil, errors.Newf("Unknown attributes type: %s", attr)
	}

	return result, nil
}

func netmaskByAddressFamily(af libipvs.AddressFamily) uint32 {
	if af == libipvs.AddressFamily(syscall.AF_INET6) {
		return 128
	}
	return 0
}

func ipToLibipvsAddressFamily(ip net.IP) libipvs.AddressFamily {
	// If ip is not an IPv4 address, To4 returns nil.
	if r := ip.To4(); r != nil {
		return libipvs.AddressFamily(syscall.AF_INET)
	}
	return libipvs.AddressFamily(syscall.AF_INET6)
}

func tokglbIPVSScheduler(scheduler string) (kglb_pb.IpvsService_Scheduler, error) {
	switch scheduler {
	case "rr":
		return kglb_pb.IpvsService_RR, nil
	case "wrr":
		return kglb_pb.IpvsService_WRR, nil
	case "ip_vs_sch":
		return kglb_pb.IpvsService_IP_VS_SCH, nil
	default:
		return 0, errors.Newf("unknown libipvs ipvs scheduler: %v", scheduler)
	}
}

func tolibIPVSScheduler(scheduler kglb_pb.IpvsService_Scheduler) (string, error) {
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

func libipvsProtocolTokglbProtocol(protocol libipvs.Protocol) (kglb_pb.IPProtocol, error) {
	switch protocol {
	case syscall.IPPROTO_TCP:
		return kglb_pb.IPProtocol_TCP, nil
	case syscall.IPPROTO_UDP:
		return kglb_pb.IPProtocol_UDP, nil
	}
	return 0, errors.Newf("unknown libipvs protocol: %v", protocol)
}

// convert libipvs forwarding method into kglb.
func tokglbUpstreamForwardMethod(
	fwdMethod libipvs.FwdMethod) (kglb_pb.ForwardMethods, error) {

	switch fwdMethod {
	case libipvs.IP_VS_CONN_F_TUNNEL:
		return kglb_pb.ForwardMethods_TUNNEL, nil
	case libipvs.IP_VS_CONN_F_MASQ:
		return kglb_pb.ForwardMethods_MASQ, nil
	default:
		return 0, errors.Newf("unknown forward method: %v", fwdMethod.String())
	}
}

// convert kglb forwarding method into libipvs specific type.
func tolibForwardMethod(
	fwdMethod kglb_pb.ForwardMethods) (libipvs.FwdMethod, error) {

	switch fwdMethod {
	case kglb_pb.ForwardMethods_TUNNEL:
		return libipvs.IP_VS_CONN_F_TUNNEL, nil
	case kglb_pb.ForwardMethods_MASQ:
		return libipvs.IP_VS_CONN_F_MASQ, nil
	default:
		return 0, errors.Newf("unknown forward method: %v", fwdMethod.String())
	}
}

// Convert libipvs address family value into kglb specific.
func tokglbAddressFamily(family libipvs.AddressFamily) kglb_pb.AddressFamily {
	switch family.String() {
	case "inet6":
		return kglb_pb.AddressFamily_AF_INET6
	default:
		return kglb_pb.AddressFamily_AF_INET
	}
}

// Convert kglb address family into libipvs specific.
func tolibAddressFamily(family kglb_pb.AddressFamily) libipvs.AddressFamily {
	switch family {
	case kglb_pb.AddressFamily_AF_INET6:
		return libipvs.AddressFamily(syscall.AF_INET6)
	default:
		return libipvs.AddressFamily(syscall.AF_INET)
	}
}
