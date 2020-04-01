package health_checker

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	hc_pb "dropbox/proto/kglb/healthchecker"
	"godropbox/errors"
)

const (
	defaultSyslogTag    = "SyslogHealthChecker"
	defaultProbeMessage = "probe message."
)

var _ HealthChecker = &SyslogChecker{}

// Syslog checker.
type SyslogChecker struct {
	params *hc_pb.SyslogCheckerAttributes

	// hostname and pid used as part of probe message sent to syslog daemon.
	hostname    string
	pid         int
	dialContext DialContextFunc
}

func NewSyslogChecker(params *hc_pb.SyslogCheckerAttributes, dialContext DialContextFunc) (*SyslogChecker, error) {
	// overriding defaultDialContext is not allowed for syslog checker
	if dialContext != nil {
		return nil, errors.New("custom dial context is not supported in SyslogChecker")
	}

	checker := &SyslogChecker{
		params:      params,
		pid:         os.Getpid(),
		dialContext: defaultDialContext,
	}

	var err error
	checker.hostname, err = os.Hostname()
	if err != nil {
		return nil, err
	}

	if params.GetPort() > 0 {
		// make conversion once instead of repeating it every Dial call.
		dstPortStr := strconv.Itoa(int(params.GetPort()))
		// dialer.
		dialer := net.Dialer{}

		checker.dialContext = func(
			ctx context.Context,
			network,
			address string) (net.Conn, error) {

			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.Wrapf(err, "fails to parse address: %s, err: ", address)
			}
			address = net.JoinHostPort(host, dstPortStr)
			return dialer.DialContext(ctx, network, address)
		}
	}

	return checker, nil
}

func (h *SyslogChecker) GetConfiguration() *hc_pb.HealthCheckerAttributes {
	return &hc_pb.HealthCheckerAttributes{
		Attributes: &hc_pb.HealthCheckerAttributes_Syslog{
			Syslog: h.params,
		},
	}
}

// Generate message in syslog format.
func (h *SyslogChecker) syslogFormat(level int, tag, msg string) []byte {
	// ensure it ends in a \n
	nl := ""
	if !strings.HasSuffix(msg, "\n") {
		nl = "\n"
	}

	timestamp := time.Now().Format(time.RFC3339)
	formattedMsg := fmt.Sprintf(
		"<%d>%s %s %s[%d]: %s%s",
		level, timestamp, h.hostname, tag, h.pid, msg, nl)
	return []byte(formattedMsg)
}

// Performs test and returns true when test was succeed.
func (h *SyslogChecker) Check(host string, port int) error {
	timeout := timeoutMsToDuration(h.params.GetCheckTimeoutMs())
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	address := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := h.dialContext(ctx, strings.ToLower(h.params.GetProtocol().String()), address)
	if err != nil {
		return err
	}
	defer conn.Close()

	// making probe message.
	probeMsg := h.syslogFormat(int(h.params.GetLogLevel()), defaultSyslogTag, defaultProbeMessage)
	// sending message, unfortunately without any reply.
	_, err = conn.Write([]byte(probeMsg))
	if err != nil {
		return err
	}

	return nil
}
