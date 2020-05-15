package health_checker

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"time"

	. "gopkg.in/check.v1"

	"dropbox/kglb/utils/concurrency"
	"dropbox/kglb/utils/fwmark"
	hc_pb "dropbox/proto/kglb/healthchecker"
	. "godropbox/gocheck2"
)

type TcpCheckerSuite struct{}

var _ = Suite(&TcpCheckerSuite{})

func (s *TcpCheckerSuite) SetUpTest(c *C) {}

func (s *TcpCheckerSuite) TestCheckHealthTcp(c *C) {
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

	params := &hc_pb.TcpCheckerAttributes{
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}
	checker, err := NewTcpChecker(params, nil)
	c.Assert(err, NoErr)

	c.Assert(checker.Check(host, port), NoErr)
}

func (s *TcpCheckerSuite) TestCheckHealthTcpRefuse(c *C) {
	// assuming noone listening on that port on test server
	port := 65533
	host := "127.0.0.1"
	params := &hc_pb.TcpCheckerAttributes{
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}
	checker, err := NewTcpChecker(params, nil)
	c.Assert(err, NoErr)

	c.Assert(checker.Check(host, port), NotNil)
}

func (s *TcpCheckerSuite) TestCheckHealthFwGen(c *C) {
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

	params := &hc_pb.TcpCheckerAttributes{
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}
	checker, err := NewTcpChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		atomic.AddUint32(&isCalled, 1)
		return fwmark.NewTcpConnection(ctx, network, net.IP{}, address, 0)
	})
	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)

	// check that FwmarkGenerator was called.
	c.Assert(int(atomic.LoadUint32(&isCalled)), Equals, 1)

	// repeating with src addr.
	params = &hc_pb.TcpCheckerAttributes{
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}
	checker, err = NewTcpChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		atomic.AddUint32(&isCalled, 1)
		return fwmark.NewTcpConnection(ctx, network, net.ParseIP("127.0.0.1"), address, 0)
	})

	c.Assert(err, NoErr)
	c.Assert(checker.Check(host, port), NoErr)

	// check that FwmarkGenerator was called.
	c.Assert(int(atomic.LoadUint32(&isCalled)), Equals, 2)
}

func (s *TcpCheckerSuite) TestIpv6(c *C) {
	server := NewBackendWithCustomAddr(
		c,
		"[::1]:15101",
		func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusOK)
		})
	defer server.Close()

	time.Sleep(50 * time.Millisecond)

	params := &hc_pb.TcpCheckerAttributes{
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}
	checker, err := NewTcpChecker(params, nil)

	c.Assert(err, NoErr)
	c.Assert(checker.Check("::1", 15101), NoErr)

	// repeating with dial context.
	checker, err = NewTcpChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		return fwmark.NewTcpConnection(ctx, network, net.IP{}, address, 0)
	})
	c.Assert(err, NoErr)
	c.Assert(checker.Check("::1", 15101), NoErr)

	// repeating with specified src addr.
	checker, err = NewTcpChecker(params, func(ctx context.Context, network, address string) (net.Conn, error) {
		return fwmark.NewTcpConnection(ctx, network, net.ParseIP("::1"), address, 0)
	})
	c.Assert(err, NoErr)
	c.Assert(checker.Check("::1", 15101), NoErr)
}

func (s *TcpCheckerSuite) TestCheckHealthTcpConcurrent(c *C) {
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

	params := &hc_pb.TcpCheckerAttributes{
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}
	checker, err := NewTcpChecker(params, nil)

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
