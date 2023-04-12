package frame

import (
	"context"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/soheilhy/cmux"
	"golang.org/x/sync/errgroup"
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
	errorGroup *errgroup.Group
	httpServer *http.Server
}

// ListenAndServe sets the address and handler on DefaultDriver's http.Server,
// then calls ListenAndServe on it.
func (dd *defaultDriver) ListenAndServe(addr string, h http.Handler) error {
	dd.httpServer.Addr = addr
	dd.httpServer.Handler = h

	dd.errorGroup.Go(dd.httpServer.ListenAndServe)
	return dd.errorGroup.Wait()
}

func (dd *defaultDriver) ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {
	dd.httpServer.Addr = addr
	dd.httpServer.Handler = h

	dd.errorGroup.Go(func() error {
		return dd.httpServer.ListenAndServeTLS(certFile, keyFile)
	})
	return dd.errorGroup.Wait()
}

func (dd *defaultDriver) Shutdown(ctx context.Context) error {
	return dd.httpServer.Shutdown(ctx)
}

type grpcDriver struct {
	corsPolicy        string
	errorGroup        *errgroup.Group
	httpServer        *http.Server
	grpcServer        *grpc.Server
	wrappedGrpcServer *grpcweb.WrappedGrpcServer
	listener          net.Listener
}

func (gd *grpcDriver) ListenAndServe(addr string, h http.Handler) error {
	var ln net.Listener
	var err error

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
		h,
		grpcweb.WithOriginFunc(func(origin string) bool { return true }),
	)

	if gd.listener != nil {
		ln = gd.listener
	} else {
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return err
		}
	}

	m := cmux.New(ln)

	var grpcL net.Listener
	grpcMatcher := cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc")
	grpcL = m.MatchWithWriters(grpcMatcher)
	anyL := m.Match(cmux.Any())

	gd.errorGroup.Go(func() error {
		return gd.grpcServer.Serve(grpcL)
	})

	gd.errorGroup.Go(func() error {
		return gd.httpServer.Serve(anyL)
	})

	gd.errorGroup.Go(m.Serve)
	return gd.errorGroup.Wait()
}

func (gd *grpcDriver) ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {
	var ln net.Listener
	var err error

	gd.httpServer.Addr = addr
	gd.httpServer.Handler = http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if gd.wrappedGrpcServer.IsGrpcWebRequest(req) {
			gd.wrappedGrpcServer.ServeHTTP(resp, req)
			return
		}
		h.ServeHTTP(resp, req)
	})

	if gd.listener != nil {
		ln = gd.listener
	} else {
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return err
		}
	}

	m := cmux.New(ln)

	var grpcL net.Listener
	grpcMatcher := cmux.HTTP2HeaderField("content-type", "application/grpc")

	grpcL = m.Match(grpcMatcher)
	anyL := m.Match(cmux.Any())

	gd.errorGroup.Go(func() error {
		return gd.grpcServer.Serve(grpcL)
	})

	gd.errorGroup.Go(func() error {
		return gd.httpServer.ServeTLS(anyL, certFile, keyFile)
	})

	gd.errorGroup.Go(m.Serve)
	return gd.errorGroup.Wait()
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

// ServerListener Option to specify user preferred listener instead of the default provided one.
func ServerListener(listener net.Listener) Option {
	return func(c *Service) {
		c.listener = listener
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
