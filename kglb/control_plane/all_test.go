package control_plane

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	. "gopkg.in/check.v1"

	kglb_pb "dropbox/proto/kglb"
)

// Bootstrap gocheck for this package.
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

// Create tcp listener with custom handler called after accepting conn.
func NewTcpBackend(c *C, handler func(conn net.Conn)) (net.Listener, string, int) {

	lAddr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	c.Assert(err, IsNil)

	listener, err := net.ListenTCP("tcp", lAddr)
	c.Assert(err, IsNil)

	// getting port
	addr := listener.Addr()
	host, portStr, err := net.SplitHostPort(addr.String())
	c.Assert(err, IsNil)
	port, err := strconv.Atoi(portStr)
	c.Assert(err, IsNil)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		handler(conn)
	}()

	return listener, host, port
}

// Mock config loader.
type mockConfigLoader struct {
	configChan chan interface{}
}

func newMockConfigLoader() *mockConfigLoader {
	return &mockConfigLoader{
		configChan: make(chan interface{}, 1),
	}
}

func (c *mockConfigLoader) UpdateConfig(config *kglb_pb.ControlPlaneConfig) {
	c.configChan <- config
}

func (c *mockConfigLoader) Updates() <-chan interface{} {
	return c.configChan
}

func (c *mockConfigLoader) Stop() {
	close(c.configChan)
}
