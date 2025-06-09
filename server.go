package frame

import (
	"context"
	"crypto/tls"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"net"
	"net/http"
)

type noopDriver struct {
}

func (t *noopDriver) ListenAndServe(_ string, _ http.Handler) error {
	return nil
}

func (t *noopDriver) Shutdown(_ context.Context) error {
	return nil
}

type defaultDriver struct {
	ctx        context.Context
	log        *Entry
	port       string
	httpServer *http.Server
	listener   net.Listener
}

func (dd *defaultDriver) Context() context.Context {
	return dd.ctx
}

func (dd *defaultDriver) tlsConfig(certPath, certKeyPath string) (*tls.Config, error) {

	if certPath == "" || certKeyPath == "" {
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(certPath, certKeyPath)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{http2.NextProtoTLS, "http/1.1"},
	}, nil
}

func (dd *defaultDriver) getListener(address, certPath, certKeyPath string, listener net.Listener) (net.Listener, error) {
	if listener != nil {
		return listener, nil
	}

	tlsConfig, err := dd.tlsConfig(certPath, certKeyPath)
	if err != nil {
		return nil, err
	}

	if tlsConfig == nil {
		return net.Listen("tcp", address)
	}

	return tls.Listen("tcp", address, tlsConfig)
}

// ListenAndServe sets the address and handler on DefaultDriver's http.Server,
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

	dd.log.Info("http server port is : %s", addr)

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

	dd.log.Info("http server port is : %s", addr)

	return dd.httpServer.Serve(ln)

}

func (dd *defaultDriver) Shutdown(ctx context.Context) error {
	return dd.httpServer.Shutdown(ctx)
}

type grpcDriver struct {
	defaultDriver
	grpcPort string

	errorChannel chan error

	grpcServer        *grpc.Server
	wrappedGrpcServer *grpcweb.WrappedGrpcServer

	grpcListener net.Listener
}

func (gd *grpcDriver) ListenAndServe(addr string, h http.Handler) error {

	gd.httpServer.Addr = addr

	gd.httpServer.Handler = http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if gd.wrappedGrpcServer.IsGrpcWebRequest(req) ||
			gd.wrappedGrpcServer.IsAcceptableGrpcCorsRequest(req) ||
			gd.wrappedGrpcServer.IsGrpcWebSocketRequest(req) {
			gd.wrappedGrpcServer.ServeHTTP(resp, req)
			return
		}
		h.ServeHTTP(resp, req)
	})

	grpcweb.WrapHandler(
		h, grpcweb.WithOriginFunc(func(origin string) bool { return true }),
	)

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

		gd.log.Info("grpc server port is : %s", gd.grpcPort)

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
	gd.log.Info("http server port is : %s", addr)

	return gd.httpServer.Serve(httpListener)
}

func (gd *grpcDriver) ListenAndServeTLS(addr, certFile, certKeyFile string, h http.Handler) error {

	gd.httpServer.Addr = addr
	gd.httpServer.Handler = http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if gd.wrappedGrpcServer.IsGrpcWebRequest(req) {
			gd.wrappedGrpcServer.ServeHTTP(resp, req)
			return
		}
		h.ServeHTTP(resp, req)
	})

	grpcweb.WrapHandler(
		h, grpcweb.WithOriginFunc(func(origin string) bool { return true }),
	)

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

		gd.log.Info("grpc server port is : %s", address)

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

	gd.log.Info("http server port is : %s", addr)

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

// GrpcServer Option to specify an instantiated grpc server
// with an implementation that can be utilized to handle incoming requests.
func GrpcServer(grpcServer *grpc.Server) Option {
	return func(c *Service) {
		c.grpcServer = grpcServer
	}
}

func EnableGrpcServerReflection() Option {
	return func(c *Service) {
		c.grpcServerEnableReflection = true
	}
}

// ServerListener Option to specify user preferred priListener instead of the default provided one.
func ServerListener(listener net.Listener) Option {
	return func(c *Service) {
		c.priListener = listener
	}
}

// GrpcServerListener Option to specify user preferred grpcListener instead of the default
// provided one. This one is mostly useful when grpc is being utilised
func GrpcServerListener(listener net.Listener) Option {
	return func(c *Service) {
		c.secListener = listener
	}
}

// GrpcPort Option to specify the grpc port for server to bind to
func GrpcPort(port string) Option {
	return func(c *Service) {
		c.grpcPort = port
	}
}

// HttpHandler Option to specify an http handler that can be used to handle inbound http requests
func HttpHandler(h http.Handler) Option {
	return func(c *Service) {
		c.handler = h
	}
}

// NoopDriver Option to force the underlying http driver to not listen on a port.
// This is mostly useful when writing tests especially against the frame service
func NoopDriver() Option {
	return func(c *Service) {
		c.driver = &noopDriver{}
	}
}
