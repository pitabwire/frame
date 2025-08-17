package frameauthorization

import (
	"context"
)

// Authorizer defines the contract for authorization functionality
type Authorizer interface {
	// HasAccess checks if a subject can perform an action on a resource
	HasAccess(ctx context.Context, action string, subject string) (bool, error)
	
	// IsEnabled returns whether authorization is enabled
	IsEnabled() bool
}

// Config defines the configuration interface for authorization
type Config interface {
	// GetAuthorizationServiceReadURI returns the read URI for the authorization service
	GetAuthorizationServiceReadURI() string
	
	// GetAuthorizationServiceWriteURI returns the write URI for the authorization service
	GetAuthorizationServiceWriteURI() string
}

// HTTPClient defines the interface for making HTTP requests
type HTTPClient interface {
	// InvokeRestService makes a REST API call and returns status, response body, and error
	InvokeRestService(ctx context.Context, method string, url string, payload interface{}, headers map[string]string) (int, []byte, error)
}

// ClaimsProvider defines the interface for getting authentication claims
type ClaimsProvider interface {
	// GetTenantID returns the tenant ID from the current context
	GetTenantID() string
	
	// GetPartitionID returns the partition ID from the current context  
	GetPartitionID() string
}

// Logger defines the logging interface needed by the authorization module
type Logger interface {
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
}
