package frame

import (
	"context"
	"fmt"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
	"google.golang.org/grpc"
	"net/http"
	"os"
	"strings"
	"time"
)

type rootHandler struct {
	handler http.Handler
	driver  *grpcDriver
}

func (rh *rootHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if r.ProtoMajor == 2 && strings.HasPrefix(
		r.Header.Get("Content-Type"), "application/grpc") {
		rh.driver.grpcServer.ServeHTTP(w, r)
	} else {
		rh.handler.ServeHTTP(w, r)
	}

}

type grpcDriver struct {
	httpServer *http.Server
	grpcServer *grpc.Server
}

func (gd *grpcDriver) ListenAndServe(addr string, h http.Handler) error {

	rootHandler := &rootHandler{
		handler: h,
		driver:  gd,
	}

	gd.httpServer.Addr = addr
	gd.httpServer.Handler = rootHandler
	return gd.httpServer.ListenAndServe()
}

func (gd *grpcDriver) ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {
	rootHandler := &rootHandler{
		handler: h,
		driver:  gd,
	}

	gd.httpServer.Addr = addr
	gd.httpServer.Handler = rootHandler
	return gd.httpServer.ListenAndServeTLS(certFile, keyFile)
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
