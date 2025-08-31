package frametests

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/util"
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
	srv *httptest.Server
}

func (t *testDriver) ListenAndServe(_ string, h http.Handler) error {

	t.srv = httptest.NewServer(h)

	return nil
}
func (t *testDriver) ListenAndServeTLS(_, _, _ string, h http.Handler) error {
	t.srv = httptest.NewTLSServer(h)
	return nil
}

func (t *testDriver) Shutdown(_ context.Context) error {
	t.srv.Close()
	return nil
}

func (t *testDriver) GetTestServer() *httptest.Server {
	return t.srv
}

// WithHttpTestDriver uses a driver, mostly useful when writing tests against the frame service.
func WithHttpTestDriver() (frame.Option, func() *httptest.Server) {
	driver := &testDriver{}
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
