package health_checker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
	. "gopkg.in/check.v1"

	"dropbox/kglb/utils/concurrency"
	"dropbox/kglb/utils/fwmark"
	hc_pb "dropbox/proto/kglb/healthchecker"
	. "godropbox/gocheck2"
)

type backendHandler struct {
	handler func(writer http.ResponseWriter, request *http.Request)
}

func (b *backendHandler) ServeHTTP(
	writer http.ResponseWriter,
	request *http.Request) {

	b.handler(writer, request)
}

type HttpCheckerSuite struct{}

var _ = Suite(&HttpCheckerSuite{})

func (s *HttpCheckerSuite) SetUpTest(c *C) {}

func (s *HttpCheckerSuite) TestCheckHealthHttp(c *C) {
	server := httptest.NewServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}
	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)

	c.Assert(checker.Check(host, port), NoErr)

	// https should fails again http
	params.Scheme = "https"
	checker, err = NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), ErrorMatches, ".*http: server gave HTTP response to HTTPS client")
}

func (s *HttpCheckerSuite) TestCheckHealthHttps(c *C) {
	server := httptest.NewTLSServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	// TODO(dkopytkov): fix it after migrating to go1.10 because of
	// https://github.com/golang/go/issues/18411.
	//caCertPool := x509.NewCertPool()
	//caCertPool.AddCert(server.Certificate())

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "https",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)
}

func (s *HttpCheckerSuite) TestCheckHealthFwGen(c *C) {
	server := httptest.NewServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	isCalled := uint32(0)
	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		atomic.AddUint32(&isCalled, 1)
		return fwmark.NewTcpConnection(ctx, network, net.IP{}, address, 0)
	})
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)

	// check that FwmarkGenerator was called.
	c.Assert(int(atomic.LoadUint32(&isCalled)), Equals, 1)

	// repeating with src addr.
	params = &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err = NewHttpChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		atomic.AddUint32(&isCalled, 1)
		return fwmark.NewTcpConnection(ctx, network, net.ParseIP("127.0.0.1"), address, 0)
	})
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)

	// check that FwmarkGenerator was called.
	c.Assert(int(atomic.LoadUint32(&isCalled)), Equals, 2)
}

func (s *HttpCheckerSuite) TestCheckHeaders(c *C) {
	server := httptest.NewServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			if req.Header.Get("test_header") != "test_value" {
				writer.WriteHeader(http.StatusInternalServerError)
			} else {
				writer.WriteHeader(http.StatusOK)
			}
		},
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		Headers:        map[string]string{"test_header": "test_value"},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)
}

func (s *HttpCheckerSuite) TestRedirect(c *C) {
	// srvR redirects to srvE which returns 200.
	srvE, addrE, portE := NewBackend(c,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	defer srvE.Close()

	srvR, addrR, portR := NewBackend(c,
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(
				w,
				r,
				fmt.Sprintf("http://%s:%d/", addrE, portE),
				http.StatusMovedPermanently)
		})
	defer srvR.Close()

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/",
		Codes:          []uint32{http.StatusMovedPermanently},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check(addrR, portR), NoErr)
}

func (s *HttpCheckerSuite) TestIpv6(c *C) {
	server := NewBackendWithCustomAddr(
		c,
		"[::1]:15100",
		func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusOK)
		})
	defer server.Close()

	time.Sleep(50 * time.Millisecond)

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		Headers:        map[string]string{"test_header": "test_value"},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check("::1", 15100), NoErr)

	// repeating with dial context.
	checker, err = NewHttpChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		return fwmark.NewTcpConnection(ctx, network, net.IP{}, address, 0)
	})
	c.Assert(err, NoErr)
	c.Assert(checker.Check("::1", 15100), NoErr)

	// repeating with specified src addr.
	checker, err = NewHttpChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		return fwmark.NewTcpConnection(ctx, network, net.ParseIP("::1"), address, 0)
	})
	c.Assert(err, NoErr)
	c.Assert(checker.Check("::1", 15100), NoErr)

}

// validate than check doesn't take more than configured Timeout.
func (s *HttpCheckerSuite) TestConnectTimeout(c *C) {
	errChan := make(chan error)
	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: 100,
	}

	checker, err := NewHttpChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			close(errChan)
		}
		return nil, fmt.Errorf("expected")
	})
	c.Assert(err, NoErr)
	startTime := time.Now()
	// assuming nobody will listen that ip:port pair on the test machine.
	err = checker.Check("127.1.0.1", 1234)
	elapsed := time.Since(startTime)
	c.Assert(err, MultilineErrorMatches, ".*context deadline exceeded")
	// check should not take more than configured 100 ms.
	c.Assert(elapsed, GreaterThan, 99*time.Millisecond)
	c.Assert(elapsed/time.Millisecond < 200, IsTrue)

	select {
	case _, ok := <-errChan:
		c.Assert(ok, IsFalse)
	case <-time.After(time.Second):
		// Timeout didn't come through context.
		c.Log("unexpectedly errChan is not closed.")
		c.Fail()
	}
}

func (s *HttpCheckerSuite) TestCheckHealthHttpConcurrent(c *C) {
	server := httptest.NewServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		Headers:        map[string]string{"test_header": "test_value"},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)

	numWorkers := 10
	errChan := make(chan error, numWorkers)
	err = concurrency.CompleteTasks(
		context.Background(),
		numWorkers,
		numWorkers,
		func(numWorker int, numTask int) {
			errChan <- checker.Check(host, port)
		})
	c.Assert(err, NoErr)

	for i := 0; i < numWorkers; i++ {
		select {
		case err, ok := <-errChan:
			c.Assert(ok, IsTrue)
			c.Assert(err, NoErr)
		case <-time.After(time.Second):
			c.Log("check took too much time...")
			c.Fail()
		}
	}
}

func (s *HttpCheckerSuite) TestCheckHealthHttpsConcurrent(c *C) {
	server := httptest.NewTLSServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "https",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)

	// --define go_race=1 makes httptest.TLSServer really slow, with 100 workers it hits 60s Timeout
	numWorkers := 10
	errChan := make(chan error, numWorkers)
	err = concurrency.CompleteTasks(
		context.Background(),
		numWorkers,
		numWorkers,
		func(numWorker int, numTask int) {
			errChan <- checker.Check(host, port)
		})
	c.Assert(err, NoErr)

	for i := 0; i < numWorkers; i++ {
		select {
		case err, ok := <-errChan:
			c.Assert(ok, IsTrue)
			c.Assert(err, NoErr)
		case <-time.After(time.Second):
			c.Log("check took too much time...")
			c.Fail()
		}
	}
}

// check https connection via http proxy
func (s *HttpCheckerSuite) TestCheckHealthHttpProxy(c *C) {
	expectedUrl, err := url.Parse("//www.dropbox.com:443")
	c.Assert(err, NoErr)
	server := httptest.NewServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			c.Assert(req.URL, DeepEqualsPretty, expectedUrl)
			c.Assert(req.Method, Equals, http.MethodConnect)
			writer.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		ProxyCheckUrl:  "https://www.dropbox.com/KgLB",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	// healthcheck fails as mock http server doesn't actually establish tcp connection to the
	//  remote server and can't handle conversation after CONNECT.
	c.Assert(checker.Check(host, port), ErrorMatches,
		".*http: server gave HTTP response to HTTPS client")
}

// test http connection over https proxy
func (s *HttpCheckerSuite) TestCheckHealthHttpsProxy(c *C) {
	expectedUrl, err := url.Parse("http://www.dropbox.com/KgLB")
	c.Assert(err, NoErr)
	server := httptest.NewTLSServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			c.Assert(req.URL, DeepEqualsPretty, expectedUrl)
			c.Assert(req.Method, Equals, http.MethodGet)
			writer.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	// TODO(dkopytkov): fix it after migrating to go1.10 because of
	// https://github.com/golang/go/issues/18411.
	//caCertPool := x509.NewCertPool()
	//caCertPool.AddCert(server.Certificate())

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.HttpCheckerAttributes{
		ProxyCheckUrl:  "http://www.dropbox.com/KgLB",
		Scheme:         "https",
		Uri:            "/proxy",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)

	// check against another http code, check should fail
	params.Codes = []uint32{http.StatusTeapot}
	checker, err = NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), MultilineErrorMatches,
		".*unexpected status code: 200")
}

func (s *HttpCheckerSuite) TestUserAgentSet(c *C) {
	server := httptest.NewServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusOK)
			c.Assert(req.Header["User-Agent"], DeepEqualsPretty, []string{userAgent})
		},
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)

}

func (s *HttpCheckerSuite) TestCustomHost(c *C) {
	hostTestValue := "custom_host_val"

	server := httptest.NewServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			if req.Host != hostTestValue {
				writer.WriteHeader(http.StatusInternalServerError)
			}
			writer.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		Headers:        map[string]string{"Host": hostTestValue},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)
}

// check that checker respects trailing slash if it's used in the config.
func (s *HttpCheckerSuite) TestTrailingSlash(c *C) {
	server := httptest.NewServer(&backendHandler{
		handler: func(writer http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/api/v1/" {
				writer.WriteHeader(http.StatusOK)
			} else {
				writer.WriteHeader(http.StatusInternalServerError)
			}
		},
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	c.Assert(err, NoErr)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, NoErr)

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "http",
		Uri:            "/api/v1/",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)

	// checking starting slash as well.
	params.Uri = "api/v1/"
	checker, err = NewHttpChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)
}

type hostPortPair struct {
	host string
	port int
}

//XXX(tehnerd): for now while we are trying to figure out how fwmark
// checks to properly close connections on ctx deadline
func (s *HttpCheckerSuite) DisableTestHttp2(c *C) {
	mux := http.NewServeMux()

	numServers := 20
	servers := make([]*httptest.Server, numServers)
	serverPorts := make([]hostPortPair, numServers)

	for i := 0; i < numServers; i++ {
		server := httptest.NewUnstartedServer(mux)
		server.TLS = &tls.Config{
			CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
			NextProtos:   []string{http2.NextProtoTLS},
		}
		server.StartTLS()
		defer server.Close()
		servers[i] = server

		host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
		c.Assert(err, NoErr)
		port, err := strconv.Atoi(portStr)
		c.Assert(err, NoErr)
		serverPorts[i] = hostPortPair{
			host: host,
			port: port,
		}
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.ProtoMajor != 2 {
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.WriteHeader(http.StatusOK)
	})

	params := &hc_pb.HttpCheckerAttributes{
		Scheme:         "https",
		Uri:            "/",
		Codes:          []uint32{http.StatusOK},
		CheckTimeoutMs: uint32(1000),
	}

	checker, err := NewHttpChecker(params, nil)
	c.Assert(err, NoErr)

	numWorkers := 5
	errChan := make(chan error, numWorkers)

	testCnt := 10
	for i := 0; i < testCnt; i++ {
		go func() {
			err := concurrency.CompleteTasks(
				context.Background(),
				numWorkers,
				numServers,
				func(numWorker int, numTask int) {
					if err := checker.Check(serverPorts[numTask].host, serverPorts[numTask].port); err != nil {
						errChan <- err
					}
				})
			if err != nil {
				errChan <- err
			} else {
				errChan <- nil
			}
		}()
	}

	for i := 0; i < testCnt; i++ {
		select {
		case err, ok := <-errChan:
			c.Assert(ok, IsTrue)
			c.Assert(err, NoErr)
		case <-time.After(10 * time.Second):
			c.Log("check took too much time...")
			c.Fail()
		}
	}
}
