package frameauthorization

import (
	"context"
)

// Option represents a service configuration option for authorization
type Option func(ctx context.Context, service ServiceRegistry)

// ServiceRegistry defines the interface for registering authorization with a service
type ServiceRegistry interface {
	SetAuthorizer(auth Authorizer)
	GetConfig() Config
	GetHTTPClient() HTTPClient
	GetLogger() Logger
}

// WithAuthorization creates an option to enable authorization functionality
func WithAuthorization() Option {
	return func(ctx context.Context, service ServiceRegistry) {
		config := service.GetConfig()
		httpClient := service.GetHTTPClient()
		logger := service.GetLogger()
		
		authorizer := NewAuthorizer(config, httpClient, logger)
		service.SetAuthorizer(authorizer)
	}
}
