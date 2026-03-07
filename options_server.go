package frame

import (
	"context"
	"net/http"

	"github.com/pitabwire/frame/server"
)

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
