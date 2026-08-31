package frametests

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/v2"
)

func GetFreePort(ctx context.Context) (int, error) {
	a, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	var l *net.TCPListener
	l, err = net.ListenTCP("tcp", a)
	if err != nil {
		return 0, err
	}
	defer util.CloseAndLogOnError(ctx, l)
	//nolint:errcheck //its generally expected to work
	return l.Addr().(*net.TCPAddr).Port, nil
}

type testDriver struct {
	mu       sync.RWMutex
	bindAddr bool
	srv      *httptest.Server
}

func (t *testDriver) ListenAndServe(addr string, h http.Handler) error {
	srv, err := t.newServer(addr, h)
	if err != nil {
		return err
	}
	srv.Start()
	t.setServer(srv)
	return nil
}

func (t *testDriver) ListenAndServeTLS(addr, _, _ string, h http.Handler) error {
	srv, err := t.newServer(addr, h)
	if err != nil {
		return err
	}
	srv.StartTLS()
	t.setServer(srv)
	return nil
}

// newServer prepares an unstarted httptest server. Drivers created with
// WithBoundHTTPTestDriver bind the address frame derived from configuration;
// otherwise httptest picks a free loopback port. Binding happens here, on the
// caller's goroutine, so listen failures are returned from Service.Run.
func (t *testDriver) newServer(addr string, h http.Handler) (*httptest.Server, error) {
	srv := httptest.NewUnstartedServer(h)
	if !t.bindAddr {
		return srv, nil
	}

	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	// Driver construction has no request context; the listener lives for the service.
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		_ = srv.Listener.Close()
		return nil, fmt.Errorf("test driver listen on %s: %w", addr, err)
	}
	_ = srv.Listener.Close()
	srv.Listener = listener
	return srv, nil
}

func (t *testDriver) setServer(srv *httptest.Server) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.srv = srv
}

func (t *testDriver) Shutdown(_ context.Context) error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.srv == nil {
		return nil
	}
	t.srv.Close()
	return nil
}

func (t *testDriver) GetTestServer() *httptest.Server {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.srv
}

// WithHTTPTestDriver uses a driver, mostly useful when writing tests against the frame service.
// The server listens on a random loopback port; read it from the returned accessor.
func WithHTTPTestDriver() (frame.Option, func() *httptest.Server) {
	driver := &testDriver{}
	return frame.WithDriver(driver), driver.GetTestServer
}

// WithBoundHTTPTestDriver is WithHTTPTestDriver bound to the service's configured
// HTTP port (HTTPServerPort) instead of a random one. Use it when peers such as an
// OAuth2 server or webhook caller are configured with the service URL before the
// service starts. Service.Run returns once the listener is bound, so startup and
// listen errors surface synchronously — no goroutine or readiness polling needed.
func WithBoundHTTPTestDriver() (frame.Option, func() *httptest.Server) {
	driver := &testDriver{bindAddr: true}
	return frame.WithDriver(driver), driver.GetTestServer
}

type noopDriver struct {
}

func (t *noopDriver) ListenAndServe(_ string, _ http.Handler) error {
	return nil
}
func (t *noopDriver) ListenAndServeTLS(_, _, _ string, _ http.Handler) error {
	return nil
}

func (t *noopDriver) Shutdown(_ context.Context) error {
	return nil
}

// WithNoopDriver uses a no-op driver, mostly useful when writing tests against the frame service.
func WithNoopDriver() frame.Option {
	return frame.WithDriver(&noopDriver{})
}
