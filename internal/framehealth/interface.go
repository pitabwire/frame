package framehealth

import (
	"context"
	"net/http"

	"google.golang.org/grpc/health/grpc_health_v1"
)

// HealthManager defines the interface for health check management
type HealthManager interface {
	// AddChecker adds a health checker to the manager
	AddChecker(checker Checker)
	
	// RemoveChecker removes a health checker from the manager
	RemoveChecker(checker Checker) bool
	
	// GetCheckers returns all registered health checkers
	GetCheckers() []Checker
	
	// CheckHealth runs all health checks and returns the overall health status
	CheckHealth() error
	
	// HandleHealth handles HTTP health check requests
	HandleHealth(w http.ResponseWriter, r *http.Request)
	
	// HandleHealthByDefault handles HTTP health check requests with default routing
	HandleHealthByDefault(w http.ResponseWriter, r *http.Request)
	
	// GetGrpcHealthServer returns a gRPC health server implementation
	GetGrpcHealthServer() grpc_health_v1.HealthServer
}

// Checker wraps the CheckHealth method.
//
// CheckHealth returns nil if the resource is healthy, or a non-nil
// error if the resource is not healthy. CheckHealth must be safe to
// call from multiple goroutines.
type Checker interface {
	CheckHealth() error
}

// CheckerFunc is an adapter type to allow the use of ordinary functions as
// health checks. If f is a function with the appropriate signature,
// CheckerFunc(f) is a Checker that calls f.
type CheckerFunc func() error

// DatabaseChecker defines interface for database health checking
type DatabaseChecker interface {
	Checker
	CheckDatabaseConnection(ctx context.Context) error
}

// QueueChecker defines interface for queue health checking
type QueueChecker interface {
	Checker
	CheckQueueConnection(ctx context.Context) error
}

// ServiceChecker defines interface for external service health checking
type ServiceChecker interface {
	Checker
	CheckServiceHealth(ctx context.Context, serviceURL string) error
}
