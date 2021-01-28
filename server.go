package frame

import (
	"context"
	"fmt"
	"github.com/cockroachdb/cmux"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

type grpcDriver struct {
	httpServer *http.Server
	grpcServer *grpc.Server
}

func (gd *grpcDriver) ListenAndServe(addr string, h http.Handler) error {

	gd.httpServer.Addr = addr
	gd.httpServer.Handler = h

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	m := cmux.New(ln)

	var grpcL net.Listener
	grpcMatcher := cmux.HTTP2HeaderField("content-type", "application/grpc")
	grpcL = m.Match(grpcMatcher)

	anyL := m.Match(cmux.Any())

	go func() {
		err := gd.grpcServer.Serve(grpcL)
		if err != nil {
			log.Printf(" ListenAndServe -- stopping grpc server because : %v", err)
		}
	}()

	go func() {
		err := gd.httpServer.Serve(anyL)
		if err != nil {
			log.Printf(" ListenAndServe -- stopping http server because : %v", err)
		}
	}()

	return m.Serve()

}

func (gd *grpcDriver) ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {

	gd.httpServer.Addr = addr
	gd.httpServer.Handler = h

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	m := cmux.New(ln)

	var grpcL net.Listener
	grpcMatcher := cmux.HTTP2HeaderField("content-type", "application/grpc")
	grpcL = m.Match(grpcMatcher)

	anyL := m.Match(cmux.Any())

	go func() {
		err := gd.grpcServer.Serve(grpcL)
		if err != nil {
			log.Printf(" ListenAndServeTLS -- stopping grpc server because : %v", err)
		}
	}()

	go func() {
		err := gd.httpServer.ServeTLS(anyL, certFile, keyFile)
		if err != nil {
			log.Printf(" ListenAndServeTLS -- stopping http server because : %v", err)
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

func GrpcServer(grpcServer *grpc.Server, h http.Handler, httpOpts *server.Options) Option {
	return func(c *Service) {

		httpOpts.Driver = &grpcDriver{
			httpServer: &http.Server{
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
				IdleTimeout:  120 * time.Second,
			},
			grpcServer: grpcServer,
		}

		if httpOpts.RequestLogger == nil {
			httpOpts.RequestLogger = requestlog.NewNCSALogger(os.Stdout, func(e error) { fmt.Println(e) })
		}

		if h == nil {
			h = http.DefaultServeMux
		}

		c.server = server.New(h, httpOpts)
	}
}

func HttpServer(h http.Handler, httpOpts *server.Options) Option {
	return func(c *Service) {

		if httpOpts.RequestLogger == nil {
			httpOpts.RequestLogger = requestlog.NewNCSALogger(os.Stdout, func(e error) { fmt.Println(e) })
		}

		if h == nil {
			h = http.DefaultServeMux
		}

		c.server = server.New(h, httpOpts)
	}
}
