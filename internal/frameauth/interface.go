package frameauth

import (
	"context"
	"net/http"

	"google.golang.org/grpc"
)

// Authenticator defines the contract for authentication functionality
type Authenticator interface {
	// Authenticate validates a JWT token and returns an updated context with claims
	Authenticate(ctx context.Context, jwtToken string, audience string, issuer string) (context.Context, error)
	
	// HTTPMiddleware returns an HTTP middleware for authentication
	HTTPMiddleware(audience string, issuer string) func(http.Handler) http.Handler
	
	// UnaryInterceptor returns a gRPC unary server interceptor for authentication
	UnaryInterceptor(audience string, issuer string) grpc.UnaryServerInterceptor
	
	// StreamInterceptor returns a gRPC stream server interceptor for authentication
	StreamInterceptor(audience string, issuer string) grpc.StreamServerInterceptor
	
	// IsEnabled returns whether authentication is enabled
	IsEnabled() bool
}

// Config defines the configuration interface for authentication
type Config interface {
	// IsRunSecurely returns whether the service should run in secure mode
	IsRunSecurely() bool
	
	// GetOauth2WellKnownJwkData returns the JWK data for JWT verification
	GetOauth2WellKnownJwkData() string
}

// Logger defines the logging interface needed by the authentication module
type Logger interface {
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
}
