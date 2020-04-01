package dns_resolver

import (
	pb "dropbox/proto/kglb"
	"godropbox/errors"
)

type dnsResolverMock struct {
	data map[string]*pb.IP
}

func NewDnsResolverMock(data map[string]*pb.IP) *dnsResolverMock {
	return &dnsResolverMock{data: data}
}

func (r *dnsResolverMock) ResolveHost(hostname string, af pb.AddressFamily) (*pb.IP, error) {
	addr, ok := r.data[hostname]
	if !ok {
		return nil, errors.Newf("Host %s not found", hostname)
	}
	return addr, nil
}

var _ DnsResolver = &dnsResolverMock{}

// Helper to mock DnsResolver interface with custom impl of ResolveHost func.
type ResolverMock struct {
	ResolverFunc func(string, pb.AddressFamily) (*pb.IP, error)
}

func (r *ResolverMock) ResolveHost(
	hostname string,
	af pb.AddressFamily) (*pb.IP, error) {

	return r.ResolverFunc(hostname, af)
}

var _ DnsResolver = &ResolverMock{}
