package health_checker

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	hc_pb "dropbox/proto/kglb/healthchecker"
	"godropbox/errors"
)

const (
	userAgent         = "KgLB healthchecker/1.0"
	hostHeaderKey     = "Host"
	defaultHttpMethod = "GET"
)

const (
	// maximum number of tls session reuse. we would force full handshake if we exceed this number
	maxReuseCounter = 10
	// number of entries in tls cache. we are using single cache object per real
	cacheCapacity = 128
)

var _ HealthChecker = &HttpChecker{}

type tlsSessionCache struct {
	lock    sync.Mutex
	counter int
	cache   tls.ClientSessionCache
}

func (s *tlsSessionCache) Get(sessionKey string) (session *tls.ClientSessionState, ok bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.counter++
	if s.counter <= maxReuseCounter {
		return s.cache.Get(sessionKey)
	}
	s.counter = 0
	return nil, false
}

func newTLSSessionCache() tls.ClientSessionCache {
	return &tlsSessionCache{
		cache: tls.NewLRUClientSessionCache(cacheCapacity),
	}
}

func (s *tlsSessionCache) Put(sessionKey string, cs *tls.ClientSessionState) {
	s.cache.Put(sessionKey, cs)
}

// http/https health checker.
type HttpChecker struct {
	params   *hc_pb.HttpCheckerAttributes
	headers  http.Header
	scheme   string
	isSecure bool

	dialContext DialContextFunc

	// current implementation of perRealSessionCaches does not support purgin.
	// that means that if we add new real there - we will never remove it from this map.
	perRealSessionCaches sync.Map
}

func NewHttpChecker(params *hc_pb.HttpCheckerAttributes, dialContext DialContextFunc) (*HttpChecker, error) {
	headers := make(http.Header)
	for k, v := range params.GetHeaders() {
		headers.Set(k, v)
	}
	headers.Set("User-Agent", userAgent)

	// assign proper scheme and explicitly creating / closing connection since
	scheme := params.GetScheme()
	if !strings.HasSuffix(scheme, "://") {
		scheme = scheme + "://"
	}

	return &HttpChecker{
		params:      params,
		dialContext: dialContext,
		headers:     headers,
		scheme:      scheme,
		isSecure:    scheme == "https://",
	}, nil
}

func (h *HttpChecker) GetConfiguration() *hc_pb.HealthCheckerAttributes {
	return &hc_pb.HealthCheckerAttributes{
		Attributes: &hc_pb.HealthCheckerAttributes_Http{
			Http: h.params,
		},
	}
}

func (h *HttpChecker) getTLSSessionForHost(key string) tls.ClientSessionCache {
	if cache, ok := h.perRealSessionCaches.Load(key); ok {
		return cache.(tls.ClientSessionCache)
	}
	cache := newTLSSessionCache()
	h.perRealSessionCaches.Store(key, cache)
	return cache
}

// Performs test and returns true when test was succeed.
func (h *HttpChecker) Check(host string, port int) error {
	timeout := timeoutMsToDuration(h.params.GetCheckTimeoutMs())
	key := fmt.Sprintf("%s-%d", host, port)
	sessionCache := h.getTLSSessionForHost(key)
	// http.Transport is very bad at releasing connections
	transport, err := createTransport(h.isSecure, h.dialContext, timeout, sessionCache)
	if err != nil {
		return errors.Wrapf(err, "failed to create http transport: ")
	}

	requestAddr := net.JoinHostPort(host, strconv.Itoa(port))
	baseURL, err := url.Parse(h.scheme + requestAddr)
	if err != nil {
		return err
	}

	var requestURL string
	// perform http request to host:port/URLPath if conf.ProxyRequestURL is not set,
	// otherwise perform a http proxy to ProxyRequestURL using host:port as http proxy
	if h.params.GetProxyCheckUrl() == "" {
		baseURL.Path = h.params.GetUri()
		requestURL = baseURL.String()
	} else {
		requestURL = h.params.GetProxyCheckUrl()
		transport.Proxy = http.ProxyURL(baseURL)
	}

	client := http.Client{
		Transport: transport,
	}

	if !h.params.GetFollowRedirects() {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	req, err := http.NewRequest(defaultHttpMethod, requestURL, nil)
	if err != nil {
		return err
	}
	req.Close = true
	if len(h.headers) > 0 {
		req.Header = h.headers
		// Host needs to be explicitly set via Host field of http.Request structure,
		// since it overrides host header value by request.Host or request.Url.Host.
		if customHost := req.Header.Get(hostHeaderKey); customHost != "" {
			req.Host = customHost
		}
	}

	// enforce Timeout for the whole request including connect and waiting
	// response.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	defer func() {
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(ioutil.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}()

	if err != nil {
		// assuming connection will be closed after GC run
		transport = nil
		return err
	}

	// host is unhealthy if it returns unexpected status code.
	for _, status := range h.params.GetCodes() {
		if resp.StatusCode == int(status) {
			// host is healthy.
			return nil
		}
	}

	return errors.Newf(
		"http health check of %s fails: unexpected status code: %d not in %v",
		requestURL, resp.StatusCode, h.params.GetCodes())
}

func createTransport(tlsEnabled bool, dialContext DialContextFunc, tlsTimeout time.Duration, cache tls.ClientSessionCache) (*http.Transport, error) {
	transport := &http.Transport{
		DisableKeepAlives: true,
		DialContext:       dialContext,
		TLSNextProto:      make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}

	// enable ssl if it's needed.
	if tlsEnabled {
		transport.TLSHandshakeTimeout = tlsTimeout

		transport.TLSClientConfig = &tls.Config{ClientSessionCache: cache}

		// TODO(dkopytkov): CA should come from proto, but repeating what we had in the past for now.
		transport.TLSClientConfig.InsecureSkipVerify = true

		// http2 transport since it may not be configured automatically because
		// of custom tls.Config and Dial.
		//if err := h2c.ConfigureTransport(transport); err != nil {
		//	return nil, errors.Wrap(err, "Failed to configure http2 for health checker: ")
		//}
	}

	return transport, nil
}
