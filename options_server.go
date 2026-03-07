package frame

import (
	"context"
	"net"
	"net/http"

	"google.golang.org/grpc"

	"github.com/pitabwire/frame/server"
)

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
		if rl, ok := h.(RouteLister); ok {
			c.routeLister = rl
		}
	}
}

// WithHTTPMiddleware registers one or more HTTP middlewares.
// Middlewares wrap the application handler in the order supplied.
func WithHTTPMiddleware(middleware ...func(http.Handler) http.Handler) Option {
	return func(_ context.Context, c *Service) {
		if len(middleware) == 0 {
			return
		}

		c.httpMW = append(c.httpMW, middleware...)
	}
}

// WithDriver setsup a driver, mostly useful when writing tests against the frame service.
func WithDriver(driver server.Driver) Option {
	return func(_ context.Context, c *Service) {
		c.driver = driver
	}
}
