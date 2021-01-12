package frame

import (
	"context"
	"github.com/soheilhy/cmux"
	"gocloud.dev/server"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"
	"time"
)

type grpcDriver struct {
	httpServer *http.Server
	grpcServer *grpc.Server
}

func (gd *grpcDriver) ListenAndServe(addr string, h http.Handler) error {

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	m := cmux.New(l)

	if gd.grpcServer != nil {

		grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))

		go func() {

			// start the server continuously to be listening client request any tym in goroutine otherwise will exit with 1 soon it is starteds
			if err := gd.grpcServer.Serve(grpcL); err != nil {
				log.Fatalf(" ListenAndServe -- failed to serve grpc: %v", err)
			}

		}()
	}

	if gd.httpServer != nil {

		httpL := m.Match(cmux.HTTP1Fast())

		go func() {

			// start the server continuously to be listening client request any tym in goroutine otherwise will exit with 1 soon it is starteds
			if err := gd.httpServer.Serve(httpL); err != nil {
				log.Fatalf(" ListenAndServe -- failed to serve http: %v", err)
			}

		}()
	}

	if gd.grpcServer != nil || gd.httpServer != nil {
		return m.Serve()
	}
	return nil
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
