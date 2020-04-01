package fwmark

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"godropbox/errors"
)

const (
	basePort = 50000
)

// wrapper on top of net.Conn to properly close undertheath socket/file
// constructed via FileConn.
type connWrap struct {
	net.Conn
	file *os.File
}

func (c *connWrap) Close() error {
	err := c.Conn.Close()
	err2 := c.file.Close()
	if err == nil {
		err = err2
	}
	return err
}

type tcpConnParams struct {
	timeout    time.Duration
	fwmark     uint32
	localIp    net.IP
	remoteIp   net.IP // required field.
	remotePort int    // required field.
}

// convert net.IP into sockaddr structure.
func toSockaddr(ip net.IP, port int) syscall.Sockaddr {
	if ip.To4() != nil {
		addrBytes := [4]byte{}
		copy(addrBytes[:], ip.To4())
		return &syscall.SockaddrInet4{Port: port, Addr: addrBytes}
	} else {
		addrBytes := [16]byte{}
		copy(addrBytes[:], ip.To16())
		return &syscall.SockaddrInet6{Port: port, Addr: addrBytes}
	}
}

// establish tcp connection via blocking connect() call with provided socket (fd).
func tcpConn(fd int, params *tcpConnParams) error {
	if len(params.remoteIp) == 0 || params.remoteIp.IsUnspecified() {
		return fmt.Errorf("remoteIp is required")
	}

	if params.remotePort == 0 {
		return fmt.Errorf("remotePort is required")
	}

	// bind local addr if it's provided.
	if len(params.localIp) != 0 && !params.localIp.IsUnspecified() {
		port := 0
		if params.fwmark != 0 {
			port = basePort + int(params.fwmark)
			// enable SO_REUSEADDR so we could use same local port for TCP sockets which goes to different
			// remote ip/port
			err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			if err != nil {
				return errors.Wrapf(
					err,
					"SetsockoptInt() so_reuseaddr fails, dst: %v:%d, local: %v ",
					params.remoteIp.String(),
					params.remotePort,
					params.localIp.String())
			}
			// set maximum outgoing segment size. we would account for aditional encap
			err = syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_MAXSEG, 1400)
			if err != nil {
				return errors.Wrapf(
					err,
					"SetsockoptInt() tcp_maxseg fails, dst: %v:%d, local: %v ",
					params.remoteIp.String(),
					params.remotePort,
					params.localIp.String())
			}
		}
		lsa := toSockaddr(params.localIp, port)
		if err := syscall.Bind(fd, lsa); err != nil {
			return errors.Wrapf(
				err,
				"Bind() fails, dst: %v:%d, local: %s, lsa: %+v",
				params.remoteIp.String(),
				params.remotePort,
				params.localIp.String(),
				lsa)
		}
	}

	if params.fwmark != 0 {
		err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_MARK, int(params.fwmark))
		if err != nil {
			return errors.Wrapf(
				err,
				"SetsockoptInt() fails, dst: %v:%d, local: %v ",
				params.remoteIp.String(),
				params.remotePort,
				params.localIp.String())
		}
	}

	if params.timeout > 0 {
		soTimeout := durationToTimeval(params.timeout)
		if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &soTimeout); err != nil {
			return errors.Wrapf(
				err,
				"fails to apply SO_SNDTIMEO, dst: %v:%d, local: %v ",
				params.remoteIp.String(),
				params.remotePort,
				params.localIp.String())
		}
		if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &soTimeout); err != nil {
			return errors.Wrapf(
				err,
				"fails to apply SO_RCVTIMEO, dst: %v:%d, local: %v ",
				params.remoteIp.String(),
				params.remotePort,
				params.localIp.String())
		}
	}

	// sockaddr structure for dst ip.
	rsa := toSockaddr(params.remoteIp, params.remotePort)

	// TODO(dkopytkov): simplify whole this logic via dialer.Control after
	// migration to go1.12
	return syscall.Connect(fd, rsa)
}

// Establishes TCP connection to the address provided in dstAddress with a socket
// marked with fwmark. When srcAddress is provided the socket will be bind on
// that address, otherwise src address will be assign automatically by OS.
func NewTcpConnection(
	ctx context.Context,
	network string,
	localIp net.IP,
	dstAddress string,
	fwmark uint32) (net.Conn, error) {

	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, errors.Newf(
			"unknown network %s, dst: %s, local_ip: %v",
			network,
			dstAddress,
			localIp.String())
	}

	raddr, err := net.ResolveTCPAddr(network, dstAddress)
	if err != nil {
		return nil, errors.Wrapf(err, "ResolveTCPAddr() fails, dst: %s: ", dstAddress)
	}

	// golang uses network "tcp" even for IPv6 connections
	af := syscall.AF_INET6
	if raddr.IP.To4() != nil {
		af = syscall.AF_INET
	}

	fd, err := syscall.Socket(af, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		return nil, errors.Wrapf(err, "Socket() fails, dst: %s: ", dstAddress)
	}

	params := tcpConnParams{
		fwmark:     fwmark,
		localIp:    localIp,
		remoteIp:   raddr.IP,
		remotePort: raddr.Port,
	}

	if deadline, ok := ctx.Deadline(); ok {
		dur := time.Until(deadline)
		if dur <= 0 {
			return nil, fmt.Errorf("deadline has already passed: dst: %v, local: %v",
				dstAddress, localIp.String())
		}

		params.timeout = dur
	}

	if err := tcpConn(fd, &params); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}

	file := os.NewFile(uintptr(fd), "")
	fileConn, err := net.FileConn(file)
	if err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}

	return &connWrap{
		Conn: fileConn,
		file: file,
	}, nil
}

// convert duration to Timeval structure.
func durationToTimeval(dur time.Duration) syscall.Timeval {
	sec := int64(dur / time.Second)
	// dur value is in nanoseconds.
	usec := (dur.Nanoseconds() % int64(time.Second)) / int64(time.Microsecond)
	return syscall.Timeval{
		Sec:  sec,
		Usec: usec,
	}
}
