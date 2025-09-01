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

type defaultDriver struct {
	ctx        context.Context
	port       string
	httpServer *http.Server
	listener   net.Listener
}

func (dd *defaultDriver) Context() context.Context {
	return dd.ctx
}

var ErrTLSPathsNotProvided = errors.New("TLS certificate path or key path not provided")

func tlsConfig(certPath, certKeyPath string) (*tls.Config, error) {
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

func getListener(ctx context.Context,
	address, certPath, certKeyPath string,
	listener net.Listener,
) (net.Listener, error) {
	if listener != nil {
		return listener, nil
	}

	tlsCfg, err := tlsConfig(certPath, certKeyPath)
	if err != nil {
		if !errors.Is(err, ErrTLSPathsNotProvided) {
			return nil, err
		}
	}

	var lc net.ListenConfig
	listener, err = lc.Listen(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	if tlsCfg == nil {
		return listener, nil
	}

	return tls.NewListener(listener, tlsCfg), nil
}

// ListenAndServe sets the address and handlers on DefaultDriver's http.Server,
// then calls ListenAndServe on it.
func (dd *defaultDriver) ListenAndServe(addr string, h http.Handler) error {
	var ln net.Listener

	dd.httpServer.Addr = addr
	dd.httpServer.Handler = h
	log := util.Log(dd.ctx).WithField("http port", addr)

	err := http2.ConfigureServer(dd.httpServer, nil)
	if err != nil {
		return err
	}

	ln, err0 := getListener(dd.ctx, addr, "", "", dd.listener)
	if err0 != nil {
		return err0
	}

	log.Info("listening on server port")

	return dd.httpServer.Serve(ln)
}

func (dd *defaultDriver) ListenAndServeTLS(addr, certPath, certKeyPath string, h http.Handler) error {
	dd.httpServer.Addr = addr
	dd.httpServer.Handler = h
	log := util.Log(dd.ctx).WithField("https port", addr)

	err := http2.ConfigureServer(dd.httpServer, nil)
	if err != nil {
		return err
	}

	ln, err0 := getListener(dd.ctx, addr, certPath, certKeyPath, dd.listener)
	if err0 != nil {
		return err0
	}

	log.Info("listening on server port")
	return dd.httpServer.Serve(ln)
}

func (dd *defaultDriver) Shutdown(ctx context.Context) error {
	return dd.httpServer.Shutdown(ctx)
}

type grpcDriver struct {
	ctx                context.Context
	internalHttpDriver ServerDriver
	grpcPort           string

	errorChannel chan error

	grpcServer *grpc.Server

	grpcListener net.Listener
}

func (gd *grpcDriver) ListenAndServe(addr string, h http.Handler) error {
	go func(address string) {
		ln, err2 := getListener(gd.ctx, address, "", "", gd.grpcListener)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}
		log := util.Log(gd.ctx).WithField("grpc port", address)
		log.Info("listening on server port")

		err2 = gd.grpcServer.Serve(ln)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}
	}(gd.grpcPort)

	return gd.internalHttpDriver.ListenAndServe(addr, h)
}

func (gd *grpcDriver) ListenAndServeTLS(addr, certFile, certKeyFile string, h http.Handler) error {
	go func(address, certPath, certKeyPath string) {
		ln, err2 := getListener(gd.ctx, address, certPath, certKeyPath, gd.grpcListener)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}

		log := util.Log(gd.ctx).WithField("grpc port", address)
		log.Info("listening on server port")

		err2 = gd.grpcServer.Serve(ln)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}
	}(gd.grpcPort, certFile, certKeyFile)

	return gd.internalHttpDriver.ListenAndServeTLS(addr, certFile, certKeyFile, h)
}

func (gd *grpcDriver) Shutdown(ctx context.Context) error {
	if gd.grpcServer != nil {
		gd.grpcServer.Stop()
	}

	if gd.internalHttpDriver != nil {
		return gd.internalHttpDriver.Shutdown(ctx)
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

// WithDriver setsup a driver, mostly useful when writing tests against the frame service.
func WithDriver(driver ServerDriver) Option {
	return func(_ context.Context, c *Service) {
		c.driver = driver
	}
}
