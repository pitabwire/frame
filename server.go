package frame

import (
	"context"
	"gocloud.dev/server"
	"google.golang.org/grpc"
	"net/http"
	"strings"
	"time"
)

type multiProtoHandler struct {
	handler http.Handler
	driver *grpcDriver
}

func (mph *multiProtoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request){

	if r.ProtoMajor == 2 && strings.HasPrefix(
		r.Header.Get("Content-Type"), "application/grpc") {
		mph.driver.grpcServer.ServeHTTP(w, r)
	} else {
		mph.handler.ServeHTTP(w, r)
	}

}

type grpcDriver struct {
	httpServer *http.Server
	grpcServer *grpc.Server
}

func (gd *grpcDriver) ListenAndServe(addr string, h http.Handler) error {

	rootHandler := &multiProtoHandler{
		handler: h,
		driver: gd,
	}

	gd.httpServer.Addr = addr
	gd.httpServer.Handler = rootHandler
	return gd.httpServer.ListenAndServe()
}

func (gd *grpcDriver) ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {
	rootHandler := &multiProtoHandler{
		handler: h,
		driver: gd,
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

func GrpcServer(grpcServer *grpc.Server, httpOpts *server.Options) Option {
	return func(c *Service) {

		httpOpts.Driver = &grpcDriver{
			httpServer: &http.Server{
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
				IdleTimeout:  120 * time.Second,
			},
			grpcServer: grpcServer,
		}

		c.server = server.New(http.DefaultServeMux, httpOpts)

	}

}

func HttpServer(httpOpts *server.Options) Option {
	return func(c *Service) {
		c.server = server.New(http.DefaultServeMux, httpOpts)
	}
}
