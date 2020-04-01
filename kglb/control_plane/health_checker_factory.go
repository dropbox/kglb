package control_plane

import (
	"context"
	"net"
	"strconv"

	"dropbox/dlog"
	"dropbox/exclog"
	"dropbox/kglb/common"
	"dropbox/kglb/utils/fwmark"
	"dropbox/kglb/utils/health_checker"
	pb "dropbox/proto/kglb"
	"godropbox/errors"
)

// Interface to create and update HealthChecker instances based on
// BalancerConfig proto.
type HealthCheckerFactory interface {
	// Returns new instance of HealthChecker by provided configuration.
	Checker(conf *pb.BalancerConfig) (health_checker.HealthChecker, error)
}

type BaseHealthCheckerFactoryParams struct {
	// Fwmark Manager.
	FwmarkManager *fwmark.Manager

	// Optional fields to explicitly specify source IP of outgoing health check
	// related connections, otherwise they will be assigned automatically by OS.
	SourceIPv4 net.IP
	SourceIPv6 net.IP
}

type baseHealthCheckerFactory struct {
	params *BaseHealthCheckerFactoryParams

	// func to establish new TCP connection to simplify testing.
	tcpConnFunc func(
		ctx context.Context,
		network string,
		localIp net.IP,
		address string,
		fwmark uint32) (net.Conn, error)
}

// Returns new instances of health checker factory.
func NewHealthCheckerFactory(params BaseHealthCheckerFactoryParams) *baseHealthCheckerFactory {
	return newHealthCheckerFactory(fwmark.NewTcpConnection, &params)
}

// Returns HealthChecker instance based on configuration.
func (b *baseHealthCheckerFactory) Checker(conf *pb.BalancerConfig) (health_checker.HealthChecker, error) {
	var err error
	var dialContext health_checker.DialContextFunc

	// create custom dialContext if fwmarks are enabled for healthchecks
	if conf.GetEnableFwmarks() {
		dialContext, err = b.fwMarkDialContext(conf.GetLbService())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create fwMarkDialContext for %+v", conf)
		}
	}

	healthChecker, err := health_checker.NewHealthChecker(conf.GetUpstreamChecker(), dialContext)
	if err != nil {
		return nil, err
	}

	return healthChecker, nil
}

// for testing.
func newHealthCheckerFactory(
	tcpConnFunc func(
		ctx context.Context,
		network string,
		localIp net.IP,
		address string,
		fwmark uint32) (net.Conn, error),
	params *BaseHealthCheckerFactoryParams) *baseHealthCheckerFactory {

	return &baseHealthCheckerFactory{
		params:      params,
		tcpConnFunc: tcpConnFunc,
	}
}

// Generate DialContext func for fwmark service based on LoadBalancerService config.
func (b *baseHealthCheckerFactory) fwMarkDialContext(lb *pb.LoadBalancerService) (
	func(
		ctx context.Context,
		network string,
		address string) (net.Conn, error), error) {

	vip, vport, err := common.GetVipFromLbService(lb)
	if err != nil {
		err = errors.Wrapf(
			err,
			"fails to extract vip from lb_service: %+v, err: ",
			lb)
		exclog.Report(err, exclog.Critical, "")
		return nil, err
	}

	// determining addr family to bind proper src addr.
	localIp := b.params.SourceIPv4
	if net.ParseIP(vip).To4() == nil {
		localIp = b.params.SourceIPv6
	}

	if localIp == nil {
		return nil, errors.Newf("Can't determine local ip for healthcheck: %+v", b.params)
	}

	// connect to VIP with fwmark when it's enabled, otherwise establish
	// connection to auto-resolved ip.
	return func(
		ctx context.Context,
		network,
		address string) (net.Conn, error) {

		host, portStr, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.Wrapf(err, "fails to parse address: %s, err: ", address)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, errors.Wrapf(err, "fails to parse address: %s, err: ", address)
		}

		fwmarkId, err := b.params.FwmarkManager.GetAllocatedFwmark(host)
		if err != nil {
			return nil, errors.Wrapf(err, "fails to get allocated fwmark for: %s, err: ", host)
		}

		// might be helpful to debug fwmark mismatch in health checks
		// and fwmark ipvs services.
		dlog.V(2).Infof("using fwmark: %d for %s:%d, vip: %s", fwmarkId, host, port, vip)

		address = net.JoinHostPort(vip, strconv.Itoa(vport))
		conn, err := b.tcpConnFunc(ctx, network, localIp, address, fwmarkId)
		if err != nil {
			// providing more details along with exact error associated with
			// connection to simplify debugging.
			err = errors.Wrapf(err,
				"fails to connect using fwmark: %d for [%s]:%d, vip: %s, err: %v",
				fwmarkId, host, port, address, err)
		}
		return conn, err
	}, nil
}

var _ HealthCheckerFactory = &baseHealthCheckerFactory{}
