package frameserver

import (
	"context"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"
)

// ServerManager defines the contract for server management functionality
type ServerManager interface {
	// Server lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	IsRunning() bool
	
	// Server information
	GetHTTPAddress() string
	GetGRPCAddress() string
	GetHTTPServer() *http.Server
	GetGRPCServer() *grpc.Server
	
	// Health and status
	IsHealthy(ctx context.Context) bool
	GetServerStats() ServerStats
}

// ServerStats provides statistics about the server
type ServerStats struct {
	HTTPAddress    string        `json:"http_address"`
	GRPCAddress    string        `json:"grpc_address"`
	StartTime      time.Time     `json:"start_time"`
	Uptime         time.Duration `json:"uptime"`
	IsRunning      bool          `json:"is_running"`
	ActiveRequests int64         `json:"active_requests"`
	TotalRequests  int64         `json:"total_requests"`
	ErrorCount     int64         `json:"error_count"`
}

// GetConnectionCount implements common.ServerStats interface
func (s ServerStats) GetConnectionCount() int {
	return int(s.ActiveRequests)
}

// GetRequestCount implements common.ServerStats interface  
func (s ServerStats) GetRequestCount() int64 {
	return s.TotalRequests
}

// HTTPServerBuilder defines the interface for building HTTP servers
type HTTPServerBuilder interface {
	// Server configuration
	WithHandler(handler http.Handler) HTTPServerBuilder
	WithMiddleware(middleware ...MiddlewareFunc) HTTPServerBuilder
	WithTimeout(read, write, idle time.Duration) HTTPServerBuilder
	WithTLS(certFile, keyFile string) HTTPServerBuilder
	WithAddress(address string) HTTPServerBuilder
	
	// Build the server
	Build() (*http.Server, error)
}

// GRPCServerBuilder defines the interface for building gRPC servers
type GRPCServerBuilder interface {
	// Server configuration
	WithUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) GRPCServerBuilder
	WithStreamInterceptors(interceptors ...grpc.StreamServerInterceptor) GRPCServerBuilder
	WithTLS(certFile, keyFile string) GRPCServerBuilder
	WithAddress(address string) GRPCServerBuilder
	WithOptions(options ...grpc.ServerOption) GRPCServerBuilder
	
	// Service registration
	WithServiceRegistrar(registrar ServiceRegistrar) GRPCServerBuilder
	
	// Build the server
	Build() (*grpc.Server, net.Listener, error)
}

// ServiceRegistrar defines the interface for registering gRPC services
type ServiceRegistrar interface {
	RegisterServices(server *grpc.Server)
}

// MiddlewareFunc defines the signature for HTTP middleware
type MiddlewareFunc func(http.Handler) http.Handler

// Config defines the configuration interface for server functionality
type Config interface {
	// HTTP server configuration
	GetHTTPPort() int
	GetHTTPAddress() string
	GetHTTPReadTimeout() time.Duration
	GetHTTPWriteTimeout() time.Duration
	GetHTTPIdleTimeout() time.Duration
	
	// gRPC server configuration
	GetGRPCPort() int
	GetGRPCAddress() string
	
	// TLS configuration
	GetTLSCertFile() string
	GetTLSKeyFile() string
	IsRunSecurely() bool
	
	// CORS configuration
	GetCORSAllowedOrigins() []string
	GetCORSAllowedMethods() []string
	GetCORSAllowedHeaders() []string
	IsCORSEnabled() bool
}

// Logger defines the logging interface needed by the server module
type Logger interface {
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
}

// MetricsCollector defines the interface for collecting server metrics
type MetricsCollector interface {
	// HTTP metrics
	RecordHTTPRequest(method, path string, statusCode int, duration time.Duration)
	RecordHTTPError(method, path string, err error)
	
	// gRPC metrics
	RecordGRPCRequest(method string, duration time.Duration)
	RecordGRPCError(method string, err error)
	
	// Server metrics
	RecordServerStarted(serverType string)
	RecordServerStopped(serverType string, duration time.Duration)
	RecordActiveConnections(count int64)
}

// Authenticator defines the interface for authentication middleware
type Authenticator interface {
	// HTTP authentication
	HTTPMiddleware() MiddlewareFunc
	
	// gRPC authentication
	UnaryInterceptor() grpc.UnaryServerInterceptor
	StreamInterceptor() grpc.StreamServerInterceptor
}

// Authorizer defines the interface for authorization middleware
type Authorizer interface {
	// Check if a request is authorized
	IsAuthorized(ctx context.Context, action, resource string) bool
	
	// HTTP authorization middleware
	HTTPMiddleware(action, resource string) MiddlewareFunc
}

// HealthChecker defines the interface for health checking
type HealthChecker interface {
	// Health check methods
	CheckHealth(ctx context.Context) HealthStatus
	RegisterHealthCheck(name string, checker func(ctx context.Context) error)
	
	// HTTP health endpoints
	HealthHandler() http.HandlerFunc
	ReadinessHandler() http.HandlerFunc
	LivenessHandler() http.HandlerFunc
}

// HealthStatus represents the health status of the server
type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks"`
	Uptime    time.Duration     `json:"uptime"`
}

// TLSManager defines the interface for TLS certificate management
type TLSManager interface {
	// Certificate management
	LoadCertificates() error
	GetCertificate() (certFile, keyFile string)
	IsSecure() bool
	
	// Certificate validation
	ValidateCertificates() error
}

// CORSManager defines the interface for CORS management
type CORSManager interface {
	// CORS middleware
	Middleware() MiddlewareFunc
	
	// CORS configuration
	IsEnabled() bool
	GetAllowedOrigins() []string
	GetAllowedMethods() []string
	GetAllowedHeaders() []string
}

// RequestTracker defines the interface for tracking active requests
type RequestTracker interface {
	// Request tracking
	TrackRequest(ctx context.Context) context.Context
	ReleaseRequest(ctx context.Context)
	
	// Statistics
	GetActiveRequests() int64
	GetTotalRequests() int64
	GetErrorCount() int64
}
