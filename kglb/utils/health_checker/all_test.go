package health_checker

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	TestingT(t)
}

type BackendHandler struct {
	handler func(writer http.ResponseWriter, request *http.Request)
}

func (b *BackendHandler) ServeHTTP(
	writer http.ResponseWriter,
	request *http.Request) {

	b.handler(writer, request)
}

// Create http test server and returns its address and port
func NewBackend(
	c *C,
	handler func(writer http.ResponseWriter,
		request *http.Request)) (*httptest.Server, string, int) {

	server := httptest.NewServer(&BackendHandler{
		handler: handler,
	})

	backendHost, backendPortStr, err := net.SplitHostPort(
		server.Listener.Addr().String())
	c.Assert(err, IsNil)
	backendPort, err := strconv.Atoi(backendPortStr)
	c.Assert(err, IsNil)
	c.Log("created backend: ", server.Listener.Addr().String())

	return server, backendHost, backendPort
}

func NewBackendWithCustomAddr(
	c *C,
	address string,
	handler func(writer http.ResponseWriter,
		request *http.Request)) *http.Server {

	srv := &http.Server{
		Addr: address,
		Handler: &BackendHandler{
			handler: handler,
		},
	}
	go func() {
		srv.ListenAndServe()
	}()

	return srv
}
