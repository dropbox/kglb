package fwmark

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"time"

	. "gopkg.in/check.v1"

	. "godropbox/gocheck2"
)

type ConnectionSuite struct{}

var _ = Suite(&ConnectionSuite{})

func (s *ConnectionSuite) SetUpSuite(c *C) {
}

func (s *ConnectionSuite) TestTcpConnectionWithTimeout(c *C) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK\n"))
		}))
	defer server.Close()

	// counter to make sure our dial was called.
	counter := 0
	client := http.Client{Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(
			ctx context.Context,
			network,
			address string) (net.Conn, error) {

			counter++
			return NewTcpConnection(ctx, network, net.IP{}, address, 0)
		},
	}}

	req, err := http.NewRequest("GET", server.URL, nil)
	c.Assert(err, NoErr)

	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	c.Assert(err, NoErr)
	obtained, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, NoErr)
	c.Assert(string(obtained), Equals, "OK\n")
	c.Assert(counter, Equals, 1)
}

func (s *ConnectionSuite) TestTcpConnectionWithHttpClient(c *C) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK\n"))
		}))
	defer server.Close()

	// counter to make sure our dial was called.
	counter := 0
	client := http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		DisableKeepAlives: true,
		DialContext: func(
			ctx context.Context,
			network,
			address string) (net.Conn, error) {

			counter++
			return NewTcpConnection(ctx, network, net.IP{}, address, 10)
		},
	}}

	r, err := client.Get(server.URL)
	c.Assert(err, NoErr)
	defer r.Body.Close()

	obtained, err := ioutil.ReadAll(r.Body)
	c.Assert(err, NoErr)
	c.Assert(string(obtained), Equals, "OK\n")
	c.Assert(counter, Equals, 1)
}

func (s *ConnectionSuite) TestTcpConnection(c *C) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, NoErr)

	conn, err := NewTcpConnection(
		context.TODO(),
		ln.Addr().Network(),
		nil,
		ln.Addr().String(),
		10)
	c.Assert(err, NoErr)

	f, err := conn.(*connWrap).Conn.(*net.TCPConn).File()
	c.Assert(err, NoErr)

	fwmark, err := syscall.GetsockoptInt(
		int(f.Fd()),
		syscall.SOL_SOCKET,
		syscall.SO_MARK)
	c.Assert(err, NoErr)
	c.Assert(fwmark, Equals, 10)
}

func (s *ConnectionSuite) TestTcpConnectionWithSrc(c *C) {
	remoteAddrChan := make(chan string, 1)
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			remoteAddr, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				close(remoteAddrChan)
			} else {
				remoteAddrChan <- remoteAddr
			}
			w.Write([]byte("OK\n"))
		}))
	defer server.Close()

	// counter to make sure our dial was called.
	counter := 0
	client := http.Client{Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(
			ctx context.Context,
			network,
			address string) (net.Conn, error) {

			counter++
			return NewTcpConnection(ctx, network, net.ParseIP("127.1.1.1"), address, 10)
		},
	}}

	r, err := client.Get(server.URL)
	c.Assert(err, NoErr)
	defer r.Body.Close()

	select {
	case addr, ok := <-remoteAddrChan:
		c.Assert(ok, IsTrue)
		c.Assert(addr, Equals, "127.1.1.1")
	case <-time.After(time.Second):
		c.Log("timeout to wait report from callback.")
		c.Fail()
	}
}

func (s *ConnectionSuite) TestTcpConnectionLeak(c *C) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("OK\n"))
		}))
	defer server.Close()

	testCnt := 100000 // the value should be bigger than max number of available ports.
	for i := 0; i < testCnt; i++ {
		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
				DisableKeepAlives: true,
				DialContext: func(
					ctx context.Context,
					network,
					address string) (net.Conn, error) {

					return NewTcpConnection(ctx, network, net.IP{}, address, 10)
				},
			},
		}

		r, err := client.Get(server.URL)
		c.Assert(err, NoErr)
		defer r.Body.Close()

		obtained, err := ioutil.ReadAll(r.Body)
		c.Assert(err, NoErr)
		c.Assert(string(obtained), Equals, "OK\n")
	}
}

func (s *ConnectionSuite) TestTcpConnectionTimeout(c *C) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	c.Assert(err, NoErr)
	defer syscall.Close(fd)
	err = syscall.Bind(fd, &syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}})
	c.Assert(err, NoErr)
	lsa, err := syscall.Getsockname(fd)
	c.Assert(err, NoErr)
	err = syscall.Listen(fd, 0)
	c.Assert(err, NoErr)

	port := lsa.(*syscall.SockaddrInet4).Port
	addrStr := fmt.Sprintf("127.0.0.1:%d", port)
	c.Logf("bound on: %d", addrStr)

	c.Logf("filling backlog")
	done1 := make(chan struct{}, 1)
	ctx1, cancel1 := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel1()
	go func() {
		_, err := NewTcpConnection(
			ctx1,
			"tcp",
			net.IP{},
			addrStr,
			0)
		c.Assert(err, NoErr)
		close(done1)
	}()

	c.Logf("waiting for the first connection")
	<-done1

	c.Logf("starting second connection that should block (since backlog is filled)")
	done2 := make(chan error, 1)
	defer close(done2)
	timeout := 100 * time.Millisecond
	ctx2, cancel2 := context.WithTimeout(context.Background(), timeout)
	defer cancel2()
	go func() {
		_, err := NewTcpConnection(
			ctx2,
			"tcp",
			net.IP{},
			addrStr,
			0)
		done2 <- err
	}()

	c.Logf("waiting for timeout")
	select {
	case err := <-done2:
		c.Assert(err, ErrorMatches, "operation now in progress")
	case <-time.After(3 * time.Second):
		c.Fail()
	}
}

func (s *ConnectionSuite) TestTcpConnInvalidVals(c *C) {
	// missed remote addr
	c.Assert(tcpConn(0, &tcpConnParams{
		localIp: net.ParseIP("10.10.10.10"),
	}), NotNil)

	// missed remote port
	c.Assert(tcpConn(0, &tcpConnParams{
		localIp:  net.ParseIP("10.10.10.10"),
		remoteIp: net.ParseIP("10.10.10.1"),
	}), NotNil)

}

func (s *ConnectionSuite) TestToSockaddr(c *C) {
	res := toSockaddr(net.ParseIP("10.10.10.1"), 10)
	v4, ok := res.(*syscall.SockaddrInet4)
	c.Assert(ok, IsTrue)
	c.Assert(v4.Port, Equals, 10)
	c.Assert(v4.Addr, DeepEqualsPretty, [4]byte{0x0a, 0x0a, 0x0a, 0x01})

	res = toSockaddr(net.ParseIP("::1"), 23)
	v6, ok := res.(*syscall.SockaddrInet6)
	c.Assert(ok, IsTrue)
	c.Assert(v6.Port, Equals, 23)
	c.Assert(v6.Addr, DeepEqualsPretty, [16]byte{
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01})
}

func (s *ConnectionSuite) TestDurationToTimeval(c *C) {
	c.Assert(durationToTimeval(time.Second), DeepEqualsPretty, syscall.Timeval{
		Sec: 1, Usec: 0,
	})

	c.Assert(durationToTimeval(time.Second+100*time.Millisecond), DeepEqualsPretty, syscall.Timeval{
		Sec: 1, Usec: 100000,
	})
	c.Assert(durationToTimeval(1000*time.Millisecond), DeepEqualsPretty, syscall.Timeval{
		Sec: 1, Usec: 0,
	})
	c.Assert(durationToTimeval(50*time.Millisecond), DeepEqualsPretty, syscall.Timeval{
		Sec: 0, Usec: 50000,
	})
}

// Checking that connWrap doesn't return error.
func (s *ConnectionSuite) TestTcpClose(c *C) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, NoErr)

	conn, err := NewTcpConnection(
		context.TODO(),
		ln.Addr().Network(),
		nil,
		ln.Addr().String(),
		10)
	c.Assert(err, NoErr)

	err = conn.Close()
	c.Assert(err, IsNil)

	// should fails with `use of closed network connection`
	err = conn.Close()
	c.Assert(err, NotNil)
}
