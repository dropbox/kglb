package health_checker

import (
	"fmt"
	"net"
	"strconv"
	"time"

	. "gopkg.in/check.v1"

	hc_pb "dropbox/proto/kglb/healthchecker"
	. "godropbox/gocheck2"
)

type SyslogCheckerSuite struct {
	syslogV4 *net.TCPListener
	syslogV6 *net.TCPListener
}

var _ = Suite(&SyslogCheckerSuite{})

func createMockSyslogd(network string, host string) (*net.TCPListener, error) {
	// find listener address and resolve host when it is needed.
	lAddr, err := net.ResolveTCPAddr(network, fmt.Sprintf("%s:%d", host, 0))
	if err != nil {
		return nil, err
	}
	tcpListener, err := net.ListenTCP(network, lAddr)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			conn, err := tcpListener.Accept()
			if err != nil {
				continue
			}
			buff := make([]byte, 512)
			n, err := conn.Read(buff)
			if err == nil {
				continue
			}
			fmt.Printf("Syslog got message: %s\n", string(buff[:n]))
		}
	}()

	return tcpListener, err
}

func hostPort(listener *net.TCPListener) (string, int, error) {
	addr := listener.Addr()
	host, ports, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(ports)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func (s *SyslogCheckerSuite) SetUpTest(c *C) {
	var err error
	s.syslogV4, err = createMockSyslogd("tcp", "127.0.0.1")
	c.Assert(err, NoErr)
	s.syslogV6, err = createMockSyslogd("tcp6", "[::1]")
	c.Assert(err, NoErr)
}

func (s *SyslogCheckerSuite) TearDownTest(c *C) {
	s.syslogV4.Close()
	s.syslogV6.Close()
}

func (s *SyslogCheckerSuite) TestTcp(c *C) {
	params := &hc_pb.SyslogCheckerAttributes{
		LogLevel:       10,
		Protocol:       hc_pb.IPProtocol_TCP,
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}

	syslogHost, syslogPort, err := hostPort(s.syslogV4)
	c.Assert(err, NoErr)

	syslogChecker, err := NewSyslogChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(syslogChecker.Check(syslogHost, syslogPort), NoErr)

	// TODO(oleg): implement test for checking timeout
	//params.CheckTimeoutMs = -1
	////check Timeout behaviour
	//syslogChecker, err = NewSyslogChecker(params, nil)
	//c.Assert(err, NoErr)
	//c.Assert(syslogChecker.Check(syslogHost, syslogPort), ErrorMatches, ".*i/o timeout")
}

func (s *SyslogCheckerSuite) TestTcp6Success(c *C) {
	params := &hc_pb.SyslogCheckerAttributes{
		LogLevel:       10,
		Protocol:       hc_pb.IPProtocol_TCP,
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}
	syslogHost, syslogPort, err := hostPort(s.syslogV6)
	c.Assert(err, NoErr)

	syslogChecker, err := NewSyslogChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(syslogChecker.Check(syslogHost, syslogPort), NoErr)
}

// TestUdpSomewhere validates that checker really sends udp packets
func (s *SyslogCheckerSuite) TestUdpSomewhere(c *C) {
	params := &hc_pb.SyslogCheckerAttributes{
		LogLevel:       10,
		Protocol:       hc_pb.IPProtocol_UDP,
		CheckTimeoutMs: uint32(time.Second / time.Millisecond),
	}
	results := make(chan string, 0)

	port := 5120
	lAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	c.Assert(err, NoErr)
	listener, err := net.ListenUDP("udp", lAddr)
	c.Assert(err, NoErr)
	defer listener.Close()

	go func() {
		buff := make([]byte, 512)
		n, err := listener.Read(buff)
		c.Assert(err, NoErr)
		results <- string(buff[:n])
	}()

	syslogChecker, err := NewSyslogChecker(params, nil)
	c.Assert(err, NoErr)
	c.Assert(syslogChecker.Check("127.0.0.1", port), NoErr)
	select {
	case sentMsg := <-results:
		c.Assert(sentMsg, Matches, "<10>.*SyslogHealthChecker.*probe message.\n")
	case <-time.After(3 * time.Second):
		c.Log("Timeout to wait probe message")
		c.Fail()
	}
}
