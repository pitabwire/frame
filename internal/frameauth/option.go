package frameauth

import (
	"context"
)

// Option represents a service configuration option for authentication
type Option func(ctx context.Context, service ServiceRegistry)

// ServiceRegistry defines the interface for registering authentication with a service
type ServiceRegistry interface {
	SetAuthenticator(auth Authenticator)
	GetConfig() Config
	GetLogger() Logger
}

// WithAuthentication creates an option to enable authentication functionality
func WithAuthentication() Option {
	return func(ctx context.Context, service ServiceRegistry) {
		config := service.GetConfig()
		logger := service.GetLogger()
		
		authenticator := NewAuthenticator(config, logger)
		service.SetAuthenticator(authenticator)
	}
}
