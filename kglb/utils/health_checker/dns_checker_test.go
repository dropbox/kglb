package health_checker

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	. "gopkg.in/check.v1"

	"dropbox/kglb/utils/fwmark"
	hc_pb "dropbox/proto/kglb/healthchecker"
	. "godropbox/gocheck2"
)

type DnsCheckerSuite struct {
}

var _ = Suite(&DnsCheckerSuite{})

// Validating Timeout functionality.
func (s *DnsCheckerSuite) TestTimeout(c *C) {
	lAddr, addrErr := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	c.Assert(addrErr, NoErr)
	listener, addrErr := net.ListenUDP("udp", lAddr)
	c.Assert(addrErr, NoErr)
	defer listener.Close()

	// getting port
	addr := listener.LocalAddr()
	host, portStr, err := net.SplitHostPort(addr.String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.DnsCheckerAttributes{
		Protocol:       hc_pb.IPProtocol_UDP,
		QueryType:      hc_pb.DnsCheckerAttributes_A,
		QueryString:    ".",
		Rcode:          0,
		CheckTimeoutMs: 1000,
	}

	checker, err := NewDnsChecker(params, nil)
	c.Assert(err, NoErr)

	startTime := time.Now()
	// making request to custom listener which does nothing with received packet.
	err = checker.Check(host, port)
	elapsed := time.Since(startTime)
	c.Assert(err, NotNil)
	c.Assert(elapsed/time.Millisecond, GreaterThan, 50)
}

// Validating custom DialContext.
func (s *DnsCheckerSuite) TestCustomDialerContext(c *C) {
	// construct dns server.
	dnsSrv := &DnsHandler{
		Handler: func(w dns.ResponseWriter, r *dns.Msg) {
			// generate response.
			r.SetRcode(r, dns.RcodeSuccess)
			w.WriteMsg(r)
		},
	}

	// start dns server.
	dnsSrv.ListenAndServe("udp", "127.1.2.3:1028")
	defer dnsSrv.Close()
	time.Sleep(50 * time.Millisecond)

	params := &hc_pb.DnsCheckerAttributes{
		Protocol:       hc_pb.IPProtocol_UDP,
		QueryType:      hc_pb.DnsCheckerAttributes_A,
		QueryString:    ".",
		Rcode:          0,
		CheckTimeoutMs: 51,
	}

	checker, err := NewDnsChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		dialer := net.Dialer{}
		return dialer.DialContext(ctx, network, "127.1.2.3:1028")
	})
	c.Assert(err, NoErr)

	err = checker.Check("1.1.1.1", 1000)
	c.Assert(err, NoErr)
}

// Validating Timeout functionality.
func (s *DnsCheckerSuite) TestBasicTcp(c *C) {
	// construct dns server.
	dnsSrv := &DnsHandler{
		Handler: func(w dns.ResponseWriter, r *dns.Msg) {
			// generate response.
			r.SetRcode(r, dns.RcodeSuccess)
			w.WriteMsg(r)
		},
	}

	// start dns server.
	dnsSrv.ListenAndServe("tcp", "127.1.2.3:1026")
	defer dnsSrv.Close()

	params := &hc_pb.DnsCheckerAttributes{
		Protocol:       hc_pb.IPProtocol_TCP,
		QueryType:      hc_pb.DnsCheckerAttributes_A,
		QueryString:    ".",
		Rcode:          0,
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewDnsChecker(params, nil)
	c.Assert(err, NoErr)

	// repeating few times since server listener may not be ready fast enough.
	for i := 0; i < 5; i++ {
		if err := checker.Check("127.1.2.3", 1026); err != nil {
			time.Sleep(50 * time.Millisecond)
		} else {
			break
		}
	}
	err = checker.Check("127.1.2.3", 1026)
	c.Assert(err, NoErr)

	checker, err = NewDnsChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		return fwmark.NewTcpConnection(ctx, network, net.IP{}, "127.1.2.3:1026", 0)
	})

	c.Assert(err, NoErr)
	err = checker.Check("127.1.2.3", 1026)
	c.Assert(err, NoErr)
}

func (s *DnsCheckerSuite) TestBasicUdp(c *C) {
	// construct dns server.
	dnsSrv := &DnsHandler{
		Handler: func(w dns.ResponseWriter, r *dns.Msg) {
			// generate response.
			r.SetRcode(r, dns.RcodeSuccess)
			w.WriteMsg(r)
		},
	}

	// start dns server.
	dnsSrv.ListenAndServe("udp", "127.1.2.3:1027")
	defer dnsSrv.Close()

	params := &hc_pb.DnsCheckerAttributes{
		Protocol:       hc_pb.IPProtocol_UDP,
		QueryType:      hc_pb.DnsCheckerAttributes_A,
		QueryString:    ".",
		Rcode:          0,
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewDnsChecker(params, nil)
	c.Assert(err, NoErr)

	// repeating few times since server listener may not be ready fast enough.
	for i := 0; i < 5; i++ {
		err := checker.Check("127.1.2.3", 1027)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
		} else {
			break
		}
	}
	err = checker.Check("127.1.2.3", 1027)
	c.Assert(err, NoErr)
}

func (s *DnsCheckerSuite) TestBasicFailure(c *C) {
	// construct dns server.
	dnsSrv := &DnsHandler{
		Handler: func(w dns.ResponseWriter, r *dns.Msg) {
			// generate response.
			r.SetRcode(r, dns.RcodeServerFailure)
			w.WriteMsg(r)
		},
	}

	// start dns server.
	dnsSrv.ListenAndServe("udp", "127.1.2.3:1028")
	defer dnsSrv.Close()

	params := &hc_pb.DnsCheckerAttributes{
		Protocol:       hc_pb.IPProtocol_UDP,
		QueryType:      hc_pb.DnsCheckerAttributes_A,
		QueryString:    ".",
		Rcode:          0,
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewDnsChecker(params, nil)
	c.Assert(err, NoErr)

	// repeating few times since server listener may not be ready fast enough.
	for i := 0; i < 5; i++ {
		err := checker.Check("127.1.2.3", 1028)
		if err != nil {
			if strings.Contains(err.Error(), "incorrect rcode") {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	err = checker.Check("127.1.2.3", 1028)
	c.Assert(strings.Contains(err.Error(), "incorrect rcode"), IsTrue)
}
