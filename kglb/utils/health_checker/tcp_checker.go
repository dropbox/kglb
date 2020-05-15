package health_checker

import (
	"context"
	"net"
	"strconv"

	hc_pb "dropbox/proto/kglb/healthchecker"
)

type TcpChecker struct {
	params      *hc_pb.TcpCheckerAttributes
	dialContext DialContextFunc
}

func NewTcpChecker(params *hc_pb.TcpCheckerAttributes, dialContext DialContextFunc) (*TcpChecker, error) {
	var dc DialContextFunc
	if dialContext == nil {
		dc = defaultDialContext
	} else {
		dc = dialContext
	}
	checker := &TcpChecker{
		dialContext: dc,
		params:      params,
	}
	return checker, nil
}

func (h *TcpChecker) Check(host string, port int) error {
	timeout := timeoutMsToDuration(h.params.GetCheckTimeoutMs())
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	address := net.JoinHostPort(host, strconv.Itoa(port))
	c, err := h.dialContext(ctx, "tcp", address)
	defer func() {
		if c != nil {
			c.Close()
		}
	}()
	return err
}

func (h *TcpChecker) GetConfiguration() *hc_pb.HealthCheckerAttributes {
	return &hc_pb.HealthCheckerAttributes{
		Attributes: &hc_pb.HealthCheckerAttributes_Tcp{
			Tcp: h.params,
		},
	}
}
