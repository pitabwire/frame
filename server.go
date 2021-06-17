package frame

import (
	"context"
	"github.com/soheilhy/cmux"
	"gocloud.dev/server"
	"google.golang.org/grpc"
	"log"
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


type grpcDriver struct {
	httpServer *http.Server
	grpcServer *grpc.Server

	listener net.Listener
}

func (gd *grpcDriver) ListenAndServe(addr string, h http.Handler) error {

	var ln net.Listener
	var err error

	gd.httpServer.Addr = addr
	gd.httpServer.Handler = h

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

	go func() {
		err := gd.grpcServer.Serve(grpcL)
		if err != nil {
			log.Printf(" ListenAndServe -- stopping grpc server because : %+v", err)
		}
	}()

	go func() {
		err := gd.httpServer.Serve(anyL)
		if err != nil {
			log.Printf(" ListenAndServe -- stopping http server because : %+v", err)
		}
	}()

	return m.Serve()

}

func (gd *grpcDriver) ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {

	var ln net.Listener
	var err error

	gd.httpServer.Addr = addr
	gd.httpServer.Handler = h

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

	go func() {
		err := gd.grpcServer.Serve(grpcL)
		if err != nil {
			log.Printf(" ListenAndServeTLS -- stopping grpc server because : %+v", err)
		}
	}()

	go func() {
		err := gd.httpServer.ServeTLS(anyL, certFile, keyFile)
		if err != nil {
			log.Printf(" ListenAndServeTLS -- stopping http server because : %+v", err)
		}
	}()

	return m.Serve()
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

func GrpcServer(grpcServer *grpc.Server) Option {
	return func(c *Service) {
		c.grpcServer = grpcServer
	}
}

func ServerListener(listener net.Listener) Option {
	return func(c *Service) {
		c.listener = listener
	}
}

func HttpHandler(h http.Handler) Option {
	return func(c *Service) {
		c.handler = h
	}
}

func HttpOptions(httpOpts *server.Options) Option {
	return func(c *Service) {
		c.serverOptions = httpOpts
	}
}

func NoopHttpOptions() Option {
	return func(c *Service) {
		nopOptions := &server.Options{
			Driver: &noopDriver{},
		}

		c.serverOptions = nopOptions
	}
}

