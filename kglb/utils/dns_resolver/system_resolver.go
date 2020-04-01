package dns_resolver

import (
	"context"
	"net"
	"time"

	kglb_pb "dropbox/proto/kglb"
	"godropbox/errors"
)

type SystemResolver struct {
	resolver       *net.Resolver
	maxResolveTime time.Duration
}

var _ DnsResolver = &SystemResolver{}

func NewSystemResolver(maxResolveTime time.Duration) (*SystemResolver, error) {
	return &SystemResolver{
		resolver:       &net.Resolver{},
		maxResolveTime: maxResolveTime,
	}, nil
}

func (s *SystemResolver) ResolveHost(hostname string, af kglb_pb.AddressFamily) (*kglb_pb.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.maxResolveTime)
	defer cancel()

	if addrs, err := s.resolver.LookupIPAddr(ctx, hostname); err != nil {
		return nil, err
	} else {
		for _, addr := range addrs {
			if af == kglb_pb.AddressFamily_AF_INET && addr.IP.To4() != nil {
				return &kglb_pb.IP{
					Address: &kglb_pb.IP_Ipv4{Ipv4: addr.String()},
				}, nil
			} else if af == kglb_pb.AddressFamily_AF_INET6 && addr.IP.To4() == nil {
				return &kglb_pb.IP{
					Address: &kglb_pb.IP_Ipv6{Ipv6: addr.String()},
				}, nil
			}
		}
	}

	return nil, errors.Newf("fails to resolve: hostname: %s, af: %s", hostname, af.String())
}
