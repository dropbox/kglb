package health_checker

import (
	"context"
	"net"
	"time"

	hc_pb "dropbox/proto/kglb/healthchecker"
	"godropbox/errors"
)

const (
	// default Timeout to perform individual health check.
	defaultTimeout = 5 * time.Second
)

// Common interface for all kind of health checkers.
type HealthChecker interface {
	// Performs test and returns true when test was succeed. It may return error
	// when test cannot be performed and as result healthy state cannot be
	// properly determined.
	Check(host string, port int) error
	// Returns current HC configuration
	GetConfiguration() *hc_pb.HealthCheckerAttributes
}

func NewHealthChecker(checker *hc_pb.UpstreamChecker, dialContext DialContextFunc) (HealthChecker, error) {
	switch attr := checker.GetChecker().GetAttributes().(type) {
	case *hc_pb.HealthCheckerAttributes_Dummy:
		return NewDummyChecker(attr.Dummy)
	case *hc_pb.HealthCheckerAttributes_Http:
		return NewHttpChecker(attr.Http, dialContext)
	case *hc_pb.HealthCheckerAttributes_Syslog:
		return NewSyslogChecker(attr.Syslog, dialContext)
	case *hc_pb.HealthCheckerAttributes_Dns:
		return NewDnsChecker(attr.Dns, dialContext)
	case *hc_pb.HealthCheckerAttributes_Tcp:
		return NewTcpChecker(attr.Tcp, dialContext)
	default:
		return nil, errors.Newf("Unknown Health Checker type: %s", attr)
	}
}

type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

var defaultDialContext DialContextFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := net.Dialer{}
	return dialer.DialContext(ctx, network, address)
}

func timeoutMsToDuration(timeoutMs uint32) time.Duration {
	if timeoutMs == 0 {
		return defaultTimeout
	}
	return time.Duration(timeoutMs) * time.Millisecond
}
