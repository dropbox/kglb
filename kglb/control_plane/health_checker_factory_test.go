package control_plane

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	. "gopkg.in/check.v1"

	"dropbox/kglb/utils/fwmark"
	"dropbox/kglb/utils/health_checker"
	pb "dropbox/proto/kglb"
	hc_pb "dropbox/proto/kglb/healthchecker"
	. "godropbox/gocheck2"
)

type HealthCheckerFactorySuite struct{}

var _ = Suite(&HealthCheckerFactorySuite{})

func (s *HealthCheckerFactorySuite) TestDummy(c *C) {
	balancerConfig := &pb.BalancerConfig{
		Name: "test-balancer-1",
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_TcpAttributes{
						TcpAttributes: &pb.IpvsTcpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Port:    80,
						},
					},
				},
			},
		},
		UpstreamChecker: &hc_pb.UpstreamChecker{
			RiseCount:  1,
			FallCount:  1,
			IntervalMs: 1,
			Checker:    dummyChecker,
		},
		EnableFwmarks: false,
	}

	factory := NewHealthCheckerFactory(
		BaseHealthCheckerFactoryParams{},
	)

	checker, err := factory.Checker(balancerConfig)
	c.Assert(err, NoErr)
	_, ok := checker.(*health_checker.DummyChecker)
	c.Assert(ok, IsTrue)
}

func (s *HealthCheckerFactorySuite) TestHttpFwmark(c *C) {
	// flag to validate if the new tcp conn func ws called.
	tcpConnCalled := uint32(0)

	server, backendHost, backendPort := NewBackend(
		c,
		func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusOK)
		})
	c.Log("backend: ", server.Listener.Addr().String())

	balancerConfig := &pb.BalancerConfig{
		Name: "test-balancer-1",
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_TcpAttributes{
						TcpAttributes: &pb.IpvsTcpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
							Port:    uint32(backendPort),
						},
					},
				},
			},
		},
		UpstreamChecker: &hc_pb.UpstreamChecker{
			RiseCount:  1,
			FallCount:  1,
			IntervalMs: 1,
			Checker: &hc_pb.HealthCheckerAttributes{
				Attributes: &hc_pb.HealthCheckerAttributes_Http{
					Http: &hc_pb.HttpCheckerAttributes{
						Scheme: "http",
						Uri:    "/",
						Codes: []uint32{
							http.StatusOK,
						},
					},
				},
			},
		},
		EnableFwmarks: false,
	}

	fakeTcpConn := func(
		ctx context.Context,
		network string,
		localIp net.IP,
		address string,
		fwmarkId uint32) (net.Conn, error) {

		atomic.AddUint32(&tcpConnCalled, 1)
		// dst should be vip:vport.
		if address != fmt.Sprintf("172.0.0.1:%d", backendPort) {
			return nil, fmt.Errorf("unexpected address: %s", address)
		}

		fwmarkExp := fwmark.GetFwmark(fwmark.FwmarkParams{
			Hostname: backendHost,
			Port:     uint32(backendPort),
			IP:       "172.0.0.1",
		})

		if fwmarkExp != fwmarkId {
			return nil,
				fmt.Errorf("unexpected fwmark: %d != %d", fwmarkExp, fwmarkId)
		}

		// use real backend address since we cannot use fwmark in the tests.
		return net.Dial("tcp", server.Listener.Addr().String())
	}
	factory := newHealthCheckerFactory(
		fakeTcpConn,
		&BaseHealthCheckerFactoryParams{})

	checker, err := factory.Checker(balancerConfig)
	c.Assert(err, NoErr)
	_, ok := checker.(*health_checker.HttpChecker)
	c.Assert(ok, IsTrue)

	// check conn without fwmarks.
	c.Assert(checker.Check(backendHost, backendPort), NoErr)
	c.Assert(int(atomic.LoadUint32(&tcpConnCalled)), Equals, 0)
}

func (s *HealthCheckerFactorySuite) TestHttpHeaders(c *C) {
	// setup backend.
	server, backendHost, backendPort := NewBackend(
		c,
		func(writer http.ResponseWriter, req *http.Request) {
			if req.Header.Get("MY_HDR") != "test_val" {
				writer.WriteHeader(http.StatusInternalServerError)
			} else {
				writer.WriteHeader(http.StatusOK)
			}
		})
	defer server.Close()

	factory := NewHealthCheckerFactory(
		BaseHealthCheckerFactoryParams{},
	)

	checker, err := factory.Checker(
		&pb.BalancerConfig{
			Name: "test-balancer-1",
			LbService: &pb.LoadBalancerService{
				Service: &pb.LoadBalancerService_IpvsService{
					IpvsService: &pb.IpvsService{
						Attributes: &pb.IpvsService_TcpAttributes{
							TcpAttributes: &pb.IpvsTcpAttributes{
								Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.0.0.1"}},
								Port:    uint32(backendPort),
							},
						},
					},
				},
			},
			UpstreamChecker: &hc_pb.UpstreamChecker{
				RiseCount:  1,
				FallCount:  1,
				IntervalMs: 1,
				Checker: &hc_pb.HealthCheckerAttributes{
					Attributes: &hc_pb.HealthCheckerAttributes_Http{
						Http: &hc_pb.HttpCheckerAttributes{
							Scheme: "http",
							Uri:    "/",
							Headers: map[string]string{
								"MY_HDR": "test_val",
							},
							Codes: []uint32{
								http.StatusOK,
							},
						},
					},
				},
			},
			EnableFwmarks: false,
		})
	c.Assert(err, NoErr)
	_, ok := checker.(*health_checker.HttpChecker)
	c.Assert(ok, IsTrue)

	// check conn without fwmarks.
	c.Assert(checker.Check(backendHost, backendPort), NoErr)
}

func (s *HealthCheckerFactorySuite) TestSyslog(c *C) {
	// flag to validate if the new tcp conn func ws called.
	tcpConnCalled := uint32(0)
	errChan := make(chan error, 1)

	// handler on server side to validate message comming from client side.
	handler := func(conn net.Conn) {
		defer conn.Close()

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err == nil {
			atomic.AddUint32(&tcpConnCalled, 1)
			if !strings.Contains(string(buf[:n]), "probe") {
				errChan <- fmt.Errorf("unexpected message")
			} else {
				errChan <- nil
			}
		} else {
			errChan <- err
		}

	}

	tcpListener, tcpAddr, tcpPort := NewTcpBackend(c, handler)
	defer tcpListener.Close()

	balancerConfig := &pb.BalancerConfig{
		Name: "test-syslog-1",
		LbService: &pb.LoadBalancerService{
			Service: &pb.LoadBalancerService_IpvsService{
				IpvsService: &pb.IpvsService{
					Attributes: &pb.IpvsService_UdpAttributes{
						UdpAttributes: &pb.IpvsUdpAttributes{
							Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: tcpAddr}},
							Port:    100,
						},
					},
				},
			},
		},
		UpstreamChecker: &hc_pb.UpstreamChecker{
			RiseCount:  1,
			FallCount:  1,
			IntervalMs: 1,
			Checker: &hc_pb.HealthCheckerAttributes{
				Attributes: &hc_pb.HealthCheckerAttributes_Syslog{
					Syslog: &hc_pb.SyslogCheckerAttributes{
						LogLevel:       10,
						Protocol:       hc_pb.IPProtocol_TCP,
						Port:           uint32(tcpPort),
						CheckTimeoutMs: 500,
					},
				},
			},
		},
		EnableFwmarks: false,
	}

	fakeTcpConn := func(
		ctx context.Context,
		network string,
		localIp net.IP,
		address string,
		fwmarkId uint32) (net.Conn, error) {

		c.Log("unexpected call")
		c.Fail()
		return nil, fmt.Errorf("unexpected call")
	}

	factory := newHealthCheckerFactory(
		fakeTcpConn,
		&BaseHealthCheckerFactoryParams{})

	checker, err := factory.Checker(balancerConfig)
	c.Assert(err, NoErr)
	_, ok := checker.(*health_checker.SyslogChecker)
	c.Assert(ok, IsTrue)

	// check conn without fwmarks.
	err = checker.Check(tcpAddr, 1024)
	c.Assert(err, NoErr)
	select {
	case err, ok := <-errChan:
		c.Assert(ok, IsTrue)
		c.Assert(err, NoErr)
	case <-time.After(5 * time.Second):
		c.Log("timeout")
		c.Fail()
	}
	c.Assert(int(atomic.LoadUint32(&tcpConnCalled)), Equals, 1)
}

func (s *HealthCheckerFactorySuite) TestDnsChecker(c *C) {
	mockStatus := uint32(dns.RcodeSuccess)
	handler := func(w dns.ResponseWriter, r *dns.Msg) {
		status := int(atomic.LoadUint32(&mockStatus))
		// generate response.
		r.SetRcode(r, status)
		w.WriteMsg(r)
	}
	// construct dns server.
	dnsUdpSrv := &health_checker.DnsHandler{Handler: handler}
	dnsUdpSrv.ListenAndServe("udp", "127.2.2.2:1030")
	defer dnsUdpSrv.Close()
	time.Sleep(50 * time.Millisecond)

	factory := NewHealthCheckerFactory(
		BaseHealthCheckerFactoryParams{},
	)

	checker, err := factory.Checker(
		&pb.BalancerConfig{
			Name: "test-dns-1",
			LbService: &pb.LoadBalancerService{
				Service: &pb.LoadBalancerService_IpvsService{
					IpvsService: &pb.IpvsService{
						Attributes: &pb.IpvsService_UdpAttributes{
							UdpAttributes: &pb.IpvsUdpAttributes{
								Address: &pb.IP{Address: &pb.IP_Ipv4{Ipv4: "172.2.2.2"}},
								Port:    uint32(1030),
							},
						},
					},
				},
			},
			UpstreamChecker: &hc_pb.UpstreamChecker{
				RiseCount:  1,
				FallCount:  1,
				IntervalMs: 1,
				Checker: &hc_pb.HealthCheckerAttributes{
					Attributes: &hc_pb.HealthCheckerAttributes_Dns{
						Dns: &hc_pb.DnsCheckerAttributes{
							Protocol:    hc_pb.IPProtocol_UDP,
							QueryType:   hc_pb.DnsCheckerAttributes_A,
							QueryString: ".",
							Rcode:       0,
						},
					},
				},
			},
			EnableFwmarks: false,
		})
	c.Assert(err, NoErr)
	_, ok := checker.(*health_checker.DnsChecker)
	c.Assert(ok, IsTrue)

	err = checker.Check("127.2.2.2", 1030)
	c.Assert(err, NoErr)

	// start failing checks.
	atomic.StoreUint32(&mockStatus, uint32(dns.RcodeServerFailure))
	err = checker.Check("127.2.2.2", 1030)
	c.Assert(err, NotNil)
}
