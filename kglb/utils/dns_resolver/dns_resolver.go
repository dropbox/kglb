package dns_resolver

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"

	"dropbox/kglb/common"
	pb "dropbox/proto/kglb"
	"godropbox/errors"
)

// NOTE: in ideal case we want to add local cache with ttl for each record, but since we use
// local unbound (with cache already) we can implement it later.

const (
	dnsTimeout = 1 * time.Second
)

type DnsResolver interface {
	ResolveHost(hostname string, af pb.AddressFamily) (*pb.IP, error)
}

type query struct {
	hostname string
	af       pb.AddressFamily
}

type dnsResolverImpl struct {
	serverAddr   string
	searchDomain string
	client       *dns.Client
	dnsCache     map[query]pb.IP
	cacheMiss    uint64
	cacheMutex   sync.RWMutex
}

func NewDnsResolver(
	addrstr string,
	searchDomain string) (*dnsResolverImpl, error) {

	searchDomain = strings.TrimPrefix(searchDomain, ".")

	return &dnsResolverImpl{
		serverAddr:   addrstr,
		searchDomain: searchDomain,
		client: &dns.Client{
			Net:          "udp",
			Timeout:      dnsTimeout,
			DialTimeout:  dnsTimeout,
			ReadTimeout:  dnsTimeout,
			WriteTimeout: dnsTimeout},
		dnsCache: make(map[query]pb.IP),
	}, nil
}

func (r *dnsResolverImpl) makeFqdn(hostname string) string {
	if !dns.IsFqdn(hostname) {
		hostname = fmt.Sprintf("%s.%s", hostname, r.searchDomain)
	}
	return dns.Fqdn(hostname)
}

func (r *dnsResolverImpl) performDnsQuery(hostname string, qType uint16) (*dns.Msg, error) {
	m := &dns.Msg{}
	m.SetQuestion(hostname, qType)

	res, _, err := r.client.Exchange(m, r.serverAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to perform dns query for %s:", hostname)
	}

	if res.Rcode != dns.RcodeSuccess {
		return nil, errors.Newf("Non-success status code from DNS: %v", res.Rcode)
	}

	if len(res.Answer) != 1 {
		return nil,
			errors.Newf("Invalid DNS responses count for %s, expected 1, obtained %d",
				hostname, len(res.Answer))
	}
	return res, nil
}

// Converts string represantation of the address into pb.IP structure, otherwise
// returns error when it's not possible.
func (r *dnsResolverImpl) strToIp(addr string) (*pb.IP, error) {
	ip := net.ParseIP(addr)
	if ip == nil {
		return nil, errors.Newf("cannot parse addr: %s", addr)
	}

	return common.NetIpToKglbAddr(ip), nil
}

func (r *dnsResolverImpl) ResolveHost(hostname string, af pb.AddressFamily) (*pb.IP, error) {
	// checking if hostname variable is already ip address.
	pbAddr, err := r.strToIp(hostname)
	if err == nil {
		switch pbAddr.GetAddress().(type) {
		case *pb.IP_Ipv4:
			if af == pb.AddressFamily_AF_INET {
				return pbAddr, nil
			}
			return nil, errors.Newf("fails to resolve (v4 -> v6): %v", hostname)
		case *pb.IP_Ipv6:
			if af == pb.AddressFamily_AF_INET6 {
				return pbAddr, nil
			}
			return nil, errors.Newf("fails to resolve (v6 -> v4): %v", hostname)
		default:
			return nil, errors.Newf("fails to resolve (unknown addr type): %v", hostname)
		}
	}

	// check if we have resolved this pair of hostname + af before. return if found
	// already resolved entry
	cacheKey := query{
		hostname: hostname,
		af:       af,
	}

	r.cacheMutex.RLock()
	if ip, exists := r.dnsCache[query{hostname: hostname, af: af}]; exists {
		r.cacheMutex.RUnlock()
		return &ip, nil
	}
	r.cacheMutex.RUnlock()

	qType := dns.TypeA
	hostname = r.makeFqdn(hostname)

	if af == pb.AddressFamily_AF_INET6 {
		qType = dns.TypeAAAA
	}

	res, err := r.performDnsQuery(hostname, qType)

	if err != nil {
		return nil, err
	}

	var addr net.IP
	var resolvedIp pb.IP

	switch af {
	case pb.AddressFamily_AF_INET:
		addr := res.Answer[0].(*dns.A).A
		resolvedIp.Address = &pb.IP_Ipv4{Ipv4: addr.String()}
	case pb.AddressFamily_AF_INET6:
		addr = res.Answer[0].(*dns.AAAA).AAAA
		resolvedIp.Address = &pb.IP_Ipv6{Ipv6: addr.String()}
	default:
		return nil, errors.Newf("Unknown AddressFamily: %v", af)
	}

	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()

	r.cacheMiss++
	r.dnsCache[cacheKey] = resolvedIp
	return &resolvedIp, nil
}
