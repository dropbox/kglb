package health_checker

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"

	hc_pb "dropbox/proto/kglb/healthchecker"
	"godropbox/errors"
)

var (
	// list of support dns query types.
	queryTypes = map[hc_pb.DnsCheckerAttributes_DNSQueryType]uint16{
		hc_pb.DnsCheckerAttributes_A: dns.TypeA,
		// TODO(oleg): keeping old behaviour, why AAAA->A?
		hc_pb.DnsCheckerAttributes_AAAA: dns.TypeA,
		hc_pb.DnsCheckerAttributes_NS:   dns.TypeNS,
		hc_pb.DnsCheckerAttributes_MX:   dns.TypeMX,
	}
)

var _ HealthChecker = &DnsChecker{}

// dns health checker.
type DnsChecker struct {
	params *hc_pb.DnsCheckerAttributes

	// hostname and pid used as part of probe message sent to syslog daemon.
	dialContext DialContextFunc
}

func NewDnsChecker(params *hc_pb.DnsCheckerAttributes, dialContext DialContextFunc) (*DnsChecker, error) {
	checker := &DnsChecker{
		params:      params,
		dialContext: defaultDialContext,
	}

	if dialContext != nil {
		checker.dialContext = dialContext
	}

	if params.GetQueryString() == "" {
		return nil, errors.Newf("QueryString is required: %+v", params)
	}

	if _, ok := queryTypes[params.GetQueryType()]; !ok {
		return nil, errors.Newf("Unknown QueryType: %+v", params)
	}

	return checker, nil
}

func (h *DnsChecker) GetConfiguration() *hc_pb.HealthCheckerAttributes {
	return &hc_pb.HealthCheckerAttributes{
		Attributes: &hc_pb.HealthCheckerAttributes_Dns{
			Dns: h.params,
		},
	}
}

func (h *DnsChecker) Check(host string, port int) error {
	timeout := timeoutMsToDuration(h.params.GetCheckTimeoutMs())
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	address := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := h.dialContext(ctx, strings.ToLower(h.params.GetProtocol().String()), address)

	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	dnsConn := &dns.Conn{Conn: conn}
	msgReq := &dns.Msg{}
	msgReq.SetQuestion(h.params.GetQueryString(), queryTypes[h.params.GetQueryType()])

	// requesting..
	if err = dnsConn.WriteMsg(msgReq); err != nil {
		return err
	}
	// handling response.
	msgRecv, err := dnsConn.ReadMsg()
	if err != nil {
		return err
	} else if msgRecv.Id != msgReq.Id {
		return errors.Newf(
			"message id in response doesn't match requested: %d != %d",
			msgRecv.Id,
			msgReq.Id)
	}

	// finally checking code.
	if msgRecv.Rcode != int(h.params.GetRcode()) {
		return errors.Newf("incorrect rcode: %d != %d", msgRecv.Rcode, h.params.GetRcode())
	}

	return nil
}
