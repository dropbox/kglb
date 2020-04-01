package common

import (
	"math"
	"net"
	"strings"

	pb "dropbox/proto/kglb"
	hc_pb "dropbox/proto/kglb/healthchecker"
	"godropbox/errors"
)

func ValidateDataPlaneState(s *pb.DataPlaneState) error {
	for _, b := range s.GetBalancers() {
		if err := ValidateBalancerState(b); err != nil {
			return errors.Wrapf(err, "Invalid BalancerState %+v", b)
		}
	}

	_, err := ValidateLinkAddresses(s.GetLinkAddresses())
	if err != nil {
		return errors.Wrapf(
			err,
			"Invalid LinkAddresses in BalancerState: %+v",
			s.GetLinkAddresses())
	}

	return nil
}

func ValidateBalancerState(s *pb.BalancerState) error {
	if len(s.GetName()) == 0 {
		return errors.New("BalancerState.Name cannot be empty")
	}

	for _, u := range s.Upstreams {
		if err := ValidateUpstreamState(u); err != nil {
			return errors.Wrapf(err, "Invalid UpstreamState %+v", u)
		}
	}

	return nil
}

func ValidateUpstreamState(s *pb.UpstreamState) error {
	if len(s.GetHostname()) == 0 {
		return errors.New("UpstreamState.Hostname cannot be empty")
	}

	if s.GetPort() == 0 && s.GetForwardMethod() != pb.ForwardMethods_TUNNEL {
		return errors.New("UpstreamState.Port cannot be equal 0")
	}

	if err := ValidateIP(s.GetAddress()); err != nil {
		return errors.Wrapf(err, "Invalid UpstreamState.Address %+v", s.GetAddress())
	}

	return nil
}

// Validate IP address defined in IP proto.
func ValidateIP(m *pb.IP) error {
	if m == nil {
		return errors.New("Message is empty")
	}

	if m.GetAddress() == nil {
		return errors.New("IP.Address cannot be empty")
	}

	switch typ := m.GetAddress().(type) {
	case *pb.IP_Ipv4:
		addr := net.ParseIP(m.GetIpv4()).To4()
		if addr == nil {
			return errors.New("IP.Address.Ipv4 is not a valid ipv4 address")
		}
	case *pb.IP_Ipv6:
		addr := net.ParseIP(m.GetIpv6()).To16()
		if addr == nil {
			return errors.New("IP.Address.Ipv6 is not a valid ipv6 address")
		}
	default:
		return errors.Newf("Unsupported IP.Address type %s", typ)
	}

	return nil
}

func ValidateControlPlaneConfig(c *pb.ControlPlaneConfig) error {
	// Map of vips and enable_fwmarks extracted from UpstreamChecker. It is used
	// to validate that all balancers with the same vip have the same value of
	// enable_fwmarks, otherwise data plane state will have the same vip inside
	// ip rules and link addresses which will break tunnelled health checks.
	fwmarkPerVipMap := make(map[string]bool)
	names := make(map[string]struct{})
	for _, b := range c.Balancers {
		if err := ValidateBalancer(b); err != nil {
			return errors.Wrapf(err, "Invalid BalancerConfig %s", b.GetName())
		}

		key, err := GetKeyFromLbService(b.GetLbService())
		if err != nil {
			return errors.Wrapf(
				err,
				"fails to extract vip:vport from balancer: %+v",
				b)
		}
		if _, ok := names[key]; ok {
			return errors.Newf("duplicate lb_service: %s", key)
		}
		names[key] = struct{}{}

		// extracting vip and enable_fwmarks.
		vip, _, err := GetVipFromLbService(b.GetLbService())
		if err != nil {
			return errors.Wrapf(
				err,
				"fails to extract vip from balancer: %+v",
				b)
		}

		fwmarkEnabled := b.GetEnableFwmarks()
		if val, ok := fwmarkPerVipMap[vip]; ok {
			if fwmarkEnabled != val {
				return errors.Wrapf(
					err,
					"inconsistent value of enable_fwmarks in multiple "+
						"BalancerConfig with the same vip: %s",
					vip)
			}
		} else {
			fwmarkPerVipMap[vip] = fwmarkEnabled
		}
	}
	return nil
}

func ValidateBalancer(c *pb.BalancerConfig) error {
	if len(c.GetName()) == 0 {
		return errors.New("Name cannot be empty")
	}
	if len(c.GetSetupName()) == 0 {
		return errors.New("SetupName cannot be empty")
	}

	if err := ValidateLbService(c.GetLbService()); err != nil {
		return errors.Wrapf(err, "Invalid BalancerConfig.LbService %+v", c.GetLbService())
	}

	if err := ValidateUpstreamChecker(c); err != nil {
		return errors.Wrapf(err, "Invalid BalancerConfig.UpstreamChecker %+v",
			c.GetUpstreamChecker())
	}

	if err := ValidateUpstreamRouting(c.GetUpstreamRouting()); err != nil {
		return errors.Wrapf(err, "Invalid BalancerConfig.UpstreamRouting %+v",
			c.GetUpstreamRouting())
	}

	if err := ValidateUpstreamDiscovery(c.GetUpstreamDiscovery()); err != nil {
		return errors.Wrapf(err, "Invalid BalancerConfig.UpstreamDiscovery %+v",
			c.GetUpstreamDiscovery())
	}

	if err := ValidateDynamicRouting(c.GetDynamicRouting()); err != nil {
		return errors.Wrapf(err, "Invalid BalancerConfig.DynamicRouting %+v",
			c.GetDynamicRouting())
	}

	return nil
}

func ValidateLbService(m *pb.LoadBalancerService) error {
	if m == nil {
		return errors.New("Message is empty")
	}

	if m.GetService() == nil {
		return errors.New("LbService.Service cannot be empty")
	}

	switch typ := m.Service.(type) {
	case *pb.LoadBalancerService_IpvsService:
		if err := ValidateIpvsService(m.GetIpvsService()); err != nil {
			return errors.Wrapf(err, "Invalid LbService.Service.IpvsService %+v",
				m.GetIpvsService())
		}
	default:
		return errors.Newf("Unsupported LoadBalancerService.Service type %s", typ)
	}

	return nil
}

func ValidateIpvsService(m *pb.IpvsService) error {
	if m == nil {
		return errors.New("Message is empty")
	}

	if m.Attributes == nil {
		return errors.New("Attributes cannot be empty")
	}

	switch attr := m.Attributes.(type) {
	case *pb.IpvsService_TcpAttributes:
		if attr.TcpAttributes == nil {
			return errors.New("TcpAttributes cannot be empty")
		}
		if attr.TcpAttributes.Port == 0 {
			return errors.New("TcpAttributes.Port cannot be 0")
		}
		if attr.TcpAttributes.Address == nil {
			return errors.New("IpvsTcp.Address cannot be empty")
		}
	case *pb.IpvsService_UdpAttributes:
		if attr.UdpAttributes == nil {
			return errors.New("UdpAttributes cannot be empty")
		}
		if attr.UdpAttributes.Port == 0 {
			return errors.New("UdpAttributes.Port cannot be 0")
		}
		if attr.UdpAttributes.Address == nil {
			return errors.New("UdpAttributes.Address cannot be empty")
		}
	case *pb.IpvsService_FwmarkAttributes:
		if attr.FwmarkAttributes == nil {
			return errors.New("IpvsFwmark cannot be empty")
		}
		if attr.FwmarkAttributes.Fwmark == 0 {
			return errors.New("IpvsFwmark.Fwmark cannot be 0")
		}
	default:
		return errors.Newf("Unsupported IpvsService.Service.Attributes type %s", attr)
	}

	return nil
}

func ValidateUpstreamChecker(c *pb.BalancerConfig) error {
	m := c.GetUpstreamChecker()
	if m == nil {
		return errors.New("Message is empty")
	}

	if m.GetChecker().Attributes == nil {
		return errors.New("UpstreamChecker.Attributes cannot be empty")
	}

	switch attr := m.GetChecker().GetAttributes().(type) {
	case *hc_pb.HealthCheckerAttributes_Dummy:
		if attr.Dummy == nil {
			return errors.New("Dummy cannot be empty")
		}
	case *hc_pb.HealthCheckerAttributes_Dns:
		if c.GetEnableFwmarks() && attr.Dns.GetProtocol() == hc_pb.IPProtocol_UDP {
			return errors.New("udp dns checker doesn't support fwmarks.")
		}
		if attr.Dns.GetQueryString() == "" {
			return errors.Newf("QueryString cannot be empty: %+v", attr)
		}
		// checking protocol specified in checker and lb_service.
		ipvsService, err := GetIpvsServiceFromLbService(c.GetLbService())
		if err != nil {
			return err
		}

		switch ipvsService.Attributes.(type) {
		case *pb.IpvsService_TcpAttributes:
		case *pb.IpvsService_UdpAttributes:
		case *pb.IpvsService_FwmarkAttributes:
		default:
			return errors.Newf(
				"Unknown ipvs service type: %+v", c)
		}
	case *hc_pb.HealthCheckerAttributes_Http:
		// TODO(verm666): validate attributes for http
	case *hc_pb.HealthCheckerAttributes_Syslog:
		if int(attr.Syslog.GetPort()) > math.MaxUint16 {
			return errors.Newf("syslog port value is out of bound: %+v", attr)
		}
	default:
		return errors.Newf("Unsupported UpstreamChecker attributes %s", attr)
	}

	return nil
}

func ValidateUpstreamRouting(m *pb.UpstreamRouting) error {
	if m == nil {
		return errors.New("UpstreamRouting is required")
	}

	// TODO(verm666): validate UpstreamRouting

	return nil
}

func ValidateUpstreamDiscovery(m *pb.UpstreamDiscovery) error {
	if m.Attributes == nil {
		return errors.New("Attributes cannot be empty")
	}

	switch attr := m.Attributes.(type) {
	case *pb.UpstreamDiscovery_StaticAttributes:
		if len(attr.StaticAttributes.Hosts) == 0 {
			return errors.New(
				"UpstreamDiscovery.StaticAttributes.Hosts cannot be empty")
		}
	case *pb.UpstreamDiscovery_MdbAttributes:
		if len(attr.MdbAttributes.Query) == 0 {
			return errors.New(
				"UpstreamDiscovery.MdbAttributes.Query cannot be empty")
		}
	default:
		return errors.Newf("Unsupported UpstreamDiscovery.Attributes type %s", attr)
	}

	return nil
}

func ValidateDynamicRouting(m *pb.DynamicRouting) error {
	if m == nil {
		return errors.New("Message is empty")
	}

	if m.GetAnnounceLimitRatio() > 1.0 {
		return errors.New("DynamicRouting.AnnounceLimitRatio cannot be gt 1.0")
	}

	switch attr := m.GetAttributes().(type) {
	case *pb.DynamicRouting_BgpAttributes:
		if err := ValidateBgpRouteAttributes(m.GetBgpAttributes()); err != nil {
			return errors.Wrapf(err, "Invalid DynamicRouting.BgpAttributes")
		}
	default:
		return errors.Newf("Unsupported DynamicRouting.Attributes type %s", attr)
	}

	return nil
}

func ValidateBgpRouteAttributes(m *pb.BgpRouteAttributes) error {
	if m == nil {
		return errors.New("Message is empty")
	}

	if m.GetLocalAsn() == 0 {
		return errors.New("BgpRoutingAttributes.LocalAsn cannot be 0")
	}

	if m.GetPeerAsn() == 0 {
		return errors.New("BgpRoutingAttributes.PeerAsn cannot be 0")
	}

	if len(m.GetCommunity()) == 0 {
		return errors.New("BgpRoutingAttributes.Community cannot be empty")
	}

	if err := ValidateIP(m.GetPrefix()); err != nil {
		return errors.Wrapf(err, "Invalid BgpRoutingAttributes.Prefix")
	}

	if m.GetPrefixlen() == 0 {
		return errors.New("BgpRoutingAttributes.Prefixlen cannot be 0")
	}

	return nil
}

// Validate set of link addresses.
func ValidateLinkAddresses(addrs []*pb.LinkAddress) (map[string]*pb.IP, error) {
	addrMap := make(map[string]*pb.IP)
	for _, addr := range addrs {
		if strings.TrimSpace(addr.GetLinkName()) == "" {
			return nil, errors.Newf("missed link_name %+v", addr)
		}
		if err := ValidateIP(addr.GetAddress()); err != nil {
			return nil, errors.Newf("incorrect ip address: %+v", addr)
		}
		if _, ok := addrMap[addr.GetAddress().String()]; ok {
			return nil, errors.Newf("duplicate link address %+v", addr)
		}
		addrMap[addr.GetAddress().String()] = addr.GetAddress()
	}

	return addrMap, nil
}
