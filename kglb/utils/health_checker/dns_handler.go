package health_checker

import (
	"github.com/miekg/dns"
)

// Simple dns server impl based on dns lib with custom handler to use in tests.
type DnsHandler struct {
	Handler func(w dns.ResponseWriter, r *dns.Msg)

	server *dns.Server
}

func (d *DnsHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	d.Handler(w, r)
}

func (d *DnsHandler) ListenAndServe(network, addr string) {
	d.server = &dns.Server{Addr: addr, Net: network, Handler: d}
	go func() {
		_ = d.server.ListenAndServe()
	}()

	return
}

// Shutdowns dns server and closes listener. Should be called after
// ListenAndServe() only.
func (d *DnsHandler) Close() {
	d.server.Shutdown()
}

var _ dns.Handler = &DnsHandler{}
