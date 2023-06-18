package frame

import (
	"context"
	"crypto/tls"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"net"
	"net/http"
)

type noopDriver struct {
}

func (t *noopDriver) ListenAndServe(addr string, h http.Handler) error {
	return nil
}

func (t *noopDriver) Shutdown(ctx context.Context) error {
	return nil
}

type defaultDriver struct {
	log        *logrus.Entry
	httpServer *http.Server
	listener   net.Listener
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

	if addr == "" {
		addr = ":http"
	}

	if dd.listener != nil {
		ln = dd.listener
	} else {
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return err
		}
	}

	dd.log.Infof("http server port is : %s", addr)

	return dd.httpServer.Serve(ln)
}

func (dd *defaultDriver) ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {
	var ln net.Listener

	dd.httpServer.Addr = addr
	dd.httpServer.Handler = h

	err := http2.ConfigureServer(dd.httpServer, nil)
	if err != nil {
		return err
	}

	if addr == "" {
		addr = ":https"
	}

	if dd.listener != nil {
		ln = dd.listener
	} else {
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return err
		}
	}

	dd.log.Infof("http server port is : %s", addr)

	return dd.httpServer.ServeTLS(ln, certFile, keyFile)

}

func (dd *defaultDriver) Shutdown(ctx context.Context) error {
	return dd.httpServer.Shutdown(ctx)
}

type grpcDriver struct {
	corsPolicy string
	grpcPort   string

	log          *logrus.Entry
	errorChannel chan error
	httpServer   *http.Server

	grpcServer        *grpc.Server
	wrappedGrpcServer *grpcweb.WrappedGrpcServer
	tlsConfig         *tls.Config
	priListener       net.Listener
	secListener       net.Listener
}

func (gd *grpcDriver) httpListener(addr string) (net.Listener, error) {
	if gd.priListener != nil {
		return gd.priListener, nil
	}
	return net.Listen("tcp", addr)

}

func (gd *grpcDriver) grpcListener() (net.Listener, error) {

	if gd.secListener != nil {
		return gd.secListener, nil
	}
	return net.Listen("tcp", gd.grpcPort)

}

func (gd *grpcDriver) ListenAndServe(addr string, h http.Handler) error {

	gd.httpServer.Addr = addr

	gd.httpServer.Handler = http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		resp.Header().Set("Access-Control-Allow-Origin", gd.corsPolicy)

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

	go func() {

		ln, err2 := gd.grpcListener()
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}

		gd.log.Infof("grpc server port is : %s", gd.grpcPort)

		err2 = gd.grpcServer.Serve(ln)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}
	}()

	go func(addr string) {

		ln, err2 := gd.httpListener(addr)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}

		gd.log.Infof("http server port is : %s", addr)
		err2 = gd.httpServer.Serve(ln)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}
	}(addr)

	return <-gd.errorChannel
}

func (gd *grpcDriver) ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {

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

	go func() {

		ln, err2 := gd.grpcListener()
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}

		gd.log.Infof("grpc server port is : %s", gd.grpcPort)

		err2 = gd.grpcServer.Serve(ln)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}
	}()

	go func() {

		ln, err2 := gd.httpListener(addr)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}

		gd.log.Infof("http server port is : %s", addr)

		err2 = gd.httpServer.ServeTLS(ln, certFile, keyFile)
		if err2 != nil {
			gd.errorChannel <- err2
			return
		}
	}()

	return <-gd.errorChannel
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

// ServerListener Option to specify user preferred priListener instead of the default provided one.
func ServerListener(listener net.Listener) Option {
	return func(c *Service) {
		c.priListener = listener
	}
}

// GrpcServerListener Option to specify user preferred secListener instead of the default
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

// CorsPolicy Option to specify the cors policy to utilize on the client
func CorsPolicy(cors string) Option {
	return func(c *Service) {
		c.corsPolicy = cors
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
