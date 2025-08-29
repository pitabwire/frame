package frame

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"

	"github.com/pitabwire/util"
	"gocloud.dev/server/driver"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
)

type ServerDriver interface {
	driver.Server
	driver.TLSServer
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

type defaultDriver struct {
	ctx        context.Context
	log        *util.LogEntry
	port       string
	httpServer *http.Server
	listener   net.Listener
}

func (dd *defaultDriver) Context() context.Context {
	return dd.ctx
}

var ErrTLSPathsNotProvided = errors.New("TLS certificate path or key path not provided")

func (dd *defaultDriver) tlsConfig(certPath, certKeyPath string) (*tls.Config, error) {
	if certPath == "" || certKeyPath == "" {
		return nil, ErrTLSPathsNotProvided
	}

	cert, err := tls.LoadX509KeyPair(certPath, certKeyPath)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{http2.NextProtoTLS, "http/1.1"},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func (dd *defaultDriver) getListener(
	address, certPath, certKeyPath string,
	listener net.Listener,
) (net.Listener, error) {
	if listener != nil {
		return listener, nil
	}

	tlsConfig, err := dd.tlsConfig(certPath, certKeyPath)
	if err != nil {
		if !errors.Is(err, ErrTLSPathsNotProvided) {
			return nil, err
		}
	}

	var lc net.ListenConfig
	listener, err = lc.Listen(dd.ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	if tlsConfig == nil {
		return listener, nil
	}

	return tls.NewListener(listener, tlsConfig), nil
}

// ListenAndServe sets the address and handlers on DefaultDriver's http.Server,
// then calls ListenAndServe on it.
func (dd *defaultDriver) ListenAndServe(addr string, h http.Handler) error {
	var ln net.Listener

	dd.httpServer.Addr = addr
	dd.httpServer.Handler = h

	err := http2.ConfigureServer(dd.httpServer, nil)
	if err != nil {
		return err
	}

	ln, err0 := dd.getListener(addr, "", "", dd.listener)
	if err0 != nil {
		return err0
	}

	dd.log.WithField("http port", addr).Info("listening on server port")

	return dd.httpServer.Serve(ln)
}

func (dd *defaultDriver) ListenAndServeTLS(addr, certPath, certKeyPath string, h http.Handler) error {
	dd.httpServer.Addr = addr
	dd.httpServer.Handler = h

	err := http2.ConfigureServer(dd.httpServer, nil)
	if err != nil {
		return err
	}

	ln, err0 := dd.getListener(addr, certPath, certKeyPath, dd.listener)
	if err0 != nil {
		return err0
	}

	dd.log.WithField("https port", addr).Info("listening on server port")
	return dd.httpServer.Serve(ln)
}

func (dd *defaultDriver) Shutdown(ctx context.Context) error {
	return dd.httpServer.Shutdown(ctx)
}

type grpcDriver struct {
	defaultDriver
	grpcPort string

	errorChannel chan error

	grpcServer *grpc.Server

	grpcListener net.Listener
}

func (gd *grpcDriver) ListenAndServe(addr string, h http.Handler) error {
	gd.httpServer.Addr = addr
	gd.httpServer.Handler = h

	err := http2.ConfigureServer(gd.httpServer, nil)
	if err != nil {
		return err
	}

	go func(address string) {
		ln, err2 := gd.getListener(address, "", "", gd.grpcListener)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}

		gd.log.WithField("grpc port", gd.grpcPort).Info("listening on server port")

		err2 = gd.grpcServer.Serve(ln)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}
	}(gd.grpcPort)

	httpListener, err0 := gd.getListener(addr, "", "", gd.listener)
	if err0 != nil {
		return err0
	}
	gd.log.WithField("http port", addr).Info("listening on server port")

	return gd.httpServer.Serve(httpListener)
}

func (gd *grpcDriver) ListenAndServeTLS(addr, certFile, certKeyFile string, h http.Handler) error {
	gd.httpServer.Addr = addr
	gd.httpServer.Handler = h

	err := http2.ConfigureServer(gd.httpServer, nil)
	if err != nil {
		return err
	}

	go func(address, certPath, certKeyPath string) {
		ln, err2 := gd.getListener(address, certPath, certKeyPath, gd.grpcListener)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}

		gd.log.WithField("grpc port", address).Info("listening on server port")

		err2 = gd.grpcServer.Serve(ln)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}
	}(gd.grpcPort, certFile, certKeyFile)

	httpListener, err0 := gd.getListener(addr, certFile, certKeyFile, gd.listener)
	if err0 != nil {
		return err0
	}

	gd.log.WithField("http port", addr).Info("listening on server port")

	return gd.httpServer.Serve(httpListener)
}

func (gd *grpcDriver) Shutdown(ctx context.Context) error {
	if gd.grpcServer != nil {
		gd.grpcServer.Stop()
	}

	if gd.httpServer != nil {
		return gd.httpServer.Shutdown(ctx)
	}
	return nil
}

// WithGRPCServer specifies an instantiated gRPC server with an implementation that can be utilized to handle incoming requests.
func WithGRPCServer(grpcServer *grpc.Server) Option {
	return func(_ context.Context, c *Service) {
		c.grpcServer = grpcServer
	}
}

// WithEnableGRPCServerReflection enables gRPC server reflection.
func WithEnableGRPCServerReflection() Option {
	return func(_ context.Context, c *Service) {
		c.grpcServerEnableReflection = true
	}
}

// WithGRPCServerListener specifies a user-preferred gRPC listener instead of the default provided one.
func WithGRPCServerListener(listener net.Listener) Option {
	return func(_ context.Context, c *Service) {
		c.grpcListener = listener
	}
}

// WithGRPCPort specifies the gRPC port for the server to bind to.
func WithGRPCPort(port string) Option {
	return func(_ context.Context, c *Service) {
		c.grpcPort = port
	}
}

// WithHTTPHandler specifies an HTTP handlers that can be used to handle inbound HTTP requests.
func WithHTTPHandler(h http.Handler) Option {
	return func(_ context.Context, c *Service) {
		c.handler = h
	}
}

// WithNoopDriver uses a no-op driver, mostly useful when writing tests against the frame service.
func WithNoopDriver() Option {
	return func(_ context.Context, c *Service) {
		c.driver = &noopDriver{}
	}
}
