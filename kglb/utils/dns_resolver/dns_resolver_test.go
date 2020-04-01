package dns_resolver

import (
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
	. "gopkg.in/check.v1"

	"dropbox/kglb/common"
	pb "dropbox/proto/kglb"
	. "godropbox/gocheck2"
)

type DnsResolverSuite struct {
	server  *dns.Server
	addrstr string
}

const testSearchDomain = "exmaple.com"

var _ = Suite(&DnsResolverSuite{})

func (s *DnsResolverSuite) SetUpTest(c *C) {
	var err error
	s.server, s.addrstr, err = runLocalUDPServer(":0")
	c.Assert(err, NoErr)
}

func (s *DnsResolverSuite) TearDownTest(c *C) {
	s.server.Shutdown()
}

func (s *DnsResolverSuite) TestDnsResolverWithCustomV4To6(c *C) {
	hostname := "test-hostname-1"
	hostnameFqdn := fmt.Sprintf("%s.%s.", hostname, testSearchDomain)

	r, err := NewDnsResolver(s.addrstr, testSearchDomain)
	c.Assert(err, NoErr)

	addr, err := r.ResolveHost(hostname, pb.AddressFamily_AF_INET)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "1.1.2.1")
	c.Assert(r.cacheMiss, Equals, uint64(1))

	// 2nd query to the same hostname. making sure that we reply from cache
	addr, err = r.ResolveHost(hostname, pb.AddressFamily_AF_INET)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "1.1.2.1")
	c.Assert(r.cacheMiss, Equals, uint64(1))

	addr, err = r.ResolveHost(hostnameFqdn, pb.AddressFamily_AF_INET)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "1.1.2.1")

	addr, err = r.ResolveHost(hostname, pb.AddressFamily_AF_INET6)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "fc00::1")

	addr, err = r.ResolveHost(hostnameFqdn, pb.AddressFamily_AF_INET6)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "fc00::1")

	// test-hostname-2 has AAAA entry only
	_, err = r.ResolveHost("test-hostname-2", pb.AddressFamily_AF_INET6)
	c.Assert(err, MultilineErrorMatches, "Invalid DNS responses count for test-hostname-2.exmaple.com., expected 1, obtained 0")

	// test-hostname3 has AAAA record only
	addr, err = r.ResolveHost("test-hostname-3", pb.AddressFamily_AF_INET6)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "fc00::3")

	// test-hostname4 has A record only and we expecting ipv4toIpv6 generated address
	_, err = r.ResolveHost("test-hostname-4", pb.AddressFamily_AF_INET6)
	c.Assert(err, MultilineErrorMatches, "Invalid DNS responses count for test-hostname-4.exmaple.com., expected 1, obtained 0")

	_, err = r.ResolveHost("test-hostname-unknown", pb.AddressFamily_AF_INET)
	c.Assert(err, MultilineErrorMatches, "Non-success status code from DNS: 3")
}

func (s *DnsResolverSuite) TestDnsResolverWithoutV4To6(c *C) {
	hostname := "test-hostname-1"
	hostnameFqdn := fmt.Sprintf("%s.%s.", hostname, testSearchDomain)

	r, err := NewDnsResolver(s.addrstr, testSearchDomain)
	c.Assert(err, NoErr)

	addr, err := r.ResolveHost(hostname, pb.AddressFamily_AF_INET)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "1.1.2.1")

	addr, err = r.ResolveHost(hostnameFqdn, pb.AddressFamily_AF_INET)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "1.1.2.1")

	addr, err = r.ResolveHost(hostname, pb.AddressFamily_AF_INET6)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "fc00::1")

	addr, err = r.ResolveHost(hostnameFqdn, pb.AddressFamily_AF_INET6)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "fc00::1")

	// Query A-only name for AAAA
	_, err = r.ResolveHost("test-hostname-2", pb.AddressFamily_AF_INET6)
	c.Assert(err, MultilineErrorMatches,
		"Invalid DNS responses count for test-hostname-2.exmaple.com., expected 1, obtained 0")

	// Query AAAA-only name for A
	_, err = r.ResolveHost("test-hostname-3", pb.AddressFamily_AF_INET)
	c.Assert(err, MultilineErrorMatches,
		"Invalid DNS responses count for test-hostname-3.exmaple.com., expected 1, obtained 0")

	// NXDOMAIN
	_, err = r.ResolveHost("test-hostname-unknown", pb.AddressFamily_AF_INET6)
	c.Assert(err, MultilineErrorMatches, "Non-success status code from DNS: 3")
}

func (s *DnsResolverSuite) TestDnsResolverTimeout(c *C) {
	// Set up a dummy UDP server that won't respond
	addr, err := net.ResolveUDPAddr("udp", ":0")
	c.Assert(err, NoErr)

	listener, err := net.ListenUDP("udp", addr)
	c.Assert(err, NoErr)
	defer listener.Close()

	addrstr := listener.LocalAddr().String()

	r, err := NewDnsResolver(addrstr, testSearchDomain)
	c.Assert(err, NoErr)

	allowable := 2 * dnsTimeout

	done := make(chan struct{})
	go func() {
		_, err = r.ResolveHost("test-hostname-1.", pb.AddressFamily_AF_INET)
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(allowable):
		c.Fail()
	}
}

func (s *DnsResolverSuite) TestMakeFqdn(c *C) {
	r, err := NewDnsResolver("1.2.3.4", "example.com")
	c.Assert(err, NoErr)

	c.Assert(r.makeFqdn("hostname"), Equals, "hostname.example.com.")
	c.Assert(r.makeFqdn("hostname."), Equals, "hostname.")

	// test with search domain with leading and trailing dots
	r, err = NewDnsResolver("1.2.3.4", ".test.example.com.")
	c.Assert(err, NoErr)

	c.Assert(r.makeFqdn("newhostname"), Equals, "newhostname.test.example.com.")
	c.Assert(r.makeFqdn("newhostname."), Equals, "newhostname.")

}

func runLocalUDPServer(laddr string) (*dns.Server, string, error) {
	pc, err := net.ListenPacket("udp", laddr)
	if err != nil {
		return nil, "", err
	}
	server := &dns.Server{PacketConn: pc, ReadTimeout: time.Hour, WriteTimeout: time.Hour, Handler: dnsServer}
	go server.ActivateAndServe()
	return server, pc.LocalAddr().String(), nil
}

type testDnsServer struct {
	dnsRecords map[string]struct {
		A    net.IP
		AAAA net.IP
	}
}

var dnsServer = testDnsServer{
	dnsRecords: map[string]struct {
		A    net.IP
		AAAA net.IP
	}{
		"test-hostname-1.exmaple.com.": {
			A:    net.ParseIP("1.1.2.1"),
			AAAA: net.ParseIP("fc00::1")},
		"test-hostname-2.exmaple.com.": {
			A: net.ParseIP("1.1.2.2")},
		"test-hostname-3.exmaple.com.": {
			AAAA: net.ParseIP("fc00::3")},
		"test-hostname-4.exmaple.com.": {
			A: net.ParseIP("1.1.2.4")},
	}}

func (s testDnsServer) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	m.Question = req.Question

	rName := req.Question[0].Name
	if addrs, ok := s.dnsRecords[rName]; ok {
		m.Rcode = dns.RcodeSuccess

		switch req.Question[0].Qtype {
		case dns.TypeA:
			if addrs.A != nil {
				m.Answer = make([]dns.RR, 1)
				m.Answer[0] = &dns.A{
					A: addrs.A,
					Hdr: dns.RR_Header{
						Name:   m.Question[0].Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    0}}
			}
		case dns.TypeAAAA:
			if addrs.AAAA != nil {
				m.Answer = make([]dns.RR, 1)
				m.Answer[0] = &dns.AAAA{
					AAAA: addrs.AAAA,
					Hdr: dns.RR_Header{
						Name:   m.Question[0].Name,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    0}}
			}
		}
	} else {
		m.Rcode = dns.RcodeNameError
	}
	fmt.Printf("DNS response for %s:\n %+v\n", rName, m)

	_ = w.WriteMsg(m)
}

func (s *DnsResolverSuite) TestResolveIpWithV4To6(c *C) {
	// point to unexistent srv since addr will be resolved without quering srv.
	r, err := NewDnsResolver("127.0.0.1:10000", testSearchDomain)
	c.Assert(err, NoErr)

	addr, err := r.ResolveHost("127.0.0.1", pb.AddressFamily_AF_INET)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "127.0.0.1")

	addr, err = r.ResolveHost("10.10.10.1", pb.AddressFamily_AF_INET)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "10.10.10.1")

	addr, err = r.ResolveHost("::1", pb.AddressFamily_AF_INET6)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "::1")

	addr, err = r.ResolveHost("10.10.10.1", pb.AddressFamily_AF_INET6)
	c.Assert(err, NotNil)
	c.Assert(addr, IsNil)

	addr, err = r.ResolveHost("::1", pb.AddressFamily_AF_INET)
	c.Assert(err, NotNil)
	c.Assert(addr, IsNil)
}

func (s *DnsResolverSuite) TestResolveIpWithoutV4To6(c *C) {
	// point to unexistent srv since addr will be resolved without quering srv.
	r, err := NewDnsResolver("127.0.0.1:10000", testSearchDomain)
	c.Assert(err, NoErr)

	addr, err := r.ResolveHost("127.0.0.1", pb.AddressFamily_AF_INET)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "127.0.0.1")

	addr, err = r.ResolveHost("10.10.10.1", pb.AddressFamily_AF_INET)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "10.10.10.1")

	addr, err = r.ResolveHost("::1", pb.AddressFamily_AF_INET6)
	c.Assert(err, NoErr)
	c.Assert(common.KglbAddrToNetIp(addr).String(), Equals, "::1")

	addr, err = r.ResolveHost("10.10.10.1", pb.AddressFamily_AF_INET6)
	c.Assert(err, NotNil)
	c.Assert(addr, IsNil)

	addr, err = r.ResolveHost("::1", pb.AddressFamily_AF_INET)
	c.Assert(err, NotNil)
	c.Assert(addr, IsNil)
}
