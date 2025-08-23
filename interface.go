package frame

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/pitabwire/util"
	"gorm.io/gorm"
)

// CoreService defines the essential service functionality that is always available
type CoreService interface {
	// Core service information
	Name() string
	Version() string
	Environment() string

	// Configuration and logging
	Config() any
	Log(ctx context.Context) *util.LogEntry

	// Service lifecycle management
	Init(ctx context.Context, opts ...Option)
	Run(ctx context.Context, address string) error
	Stop(ctx context.Context)

	// Extensibility hooks
	AddPreStartMethod(f func(ctx context.Context, s Service))
	AddCleanupMethod(f func(ctx context.Context))
}

// DatastoreService provides database access functionality
type DatastoreService interface {
	// Database connection management
	DBPool(name ...string) interface{}
	DB(ctx context.Context, readOnly bool) *gorm.DB
	DBWithName(ctx context.Context, name string, readOnly bool) *gorm.DB
}

// ModuleService provides module registry functionality
type ModuleService interface {
	// Module registry access
	Modules() *ModuleRegistry
	
	// Module retrieval by type
	GetModule(moduleType ModuleType) Module
	
	// Typed module retrieval with interface casting
	GetTypedModule(moduleType ModuleType, target interface{}) bool
	
	// Register a new module
	RegisterModule(module Module) error
	
	// Check if a module is available and enabled
	HasModule(moduleType ModuleType) bool
}

// LegacyService provides backward compatibility for existing functionality
type LegacyService interface {
	// JWT client management (for backward compatibility)
	JwtClient() map[string]any
	SetJwtClient(jwtCli map[string]any)
	JwtClientID() string
	JwtClientSecret() string
	
	// HTTP handler access (for backward compatibility)
	H() http.Handler
	
	// Health checking (for backward compatibility)
	HandleHealth(w http.ResponseWriter, r *http.Request)
	AddHealthCheck(checker interface{})
	
	// REST service invocation (for backward compatibility)
	InvokeRestService(ctx context.Context, method string, endpointURL string, payload map[string]any, headers map[string][]string) (int, []byte, error)
	InvokeRestServiceURLEncoded(ctx context.Context, method string, endpointURL string, payload url.Values, headers map[string]string) (int, []byte, error)
}

// Service defines the main service interface using the plugin registry pattern.
// Modules are dynamically registered and can be retrieved by type.
// This provides a clean, extensible architecture where the service doesn't need
// to know about specific modules beforehand.
type Service interface {
	// Core functionality (always available)
	CoreService
	
	// Module registry functionality
	ModuleService
	
	// Datastore functionality (for backward compatibility)
	DatastoreService
	
	// Legacy functionality (for backward compatibility)
	LegacyService
}

// Option defines a function type for configuring Service instances.
// Options are applied during service initialization to customize behavior.
type Option func(ctx context.Context, service Service)

// ModuleConfig defines the interface for detecting which modules should be enabled
type ModuleConfig interface {
	// Module enablement detection
	IsAuthenticationEnabled() bool
	IsAuthorizationEnabled() bool
	IsDataEnabled() bool
	IsQueueEnabled() bool
	IsObservabilityEnabled() bool
	IsServerEnabled() bool
}

// JobResultPipe represents a job result pipe for framedata
type JobResultPipe interface{}

// DataSource represents a data source configuration for frametests
type DataSource interface {
	IsDB() bool
	IsCache() bool
	IsQueue() bool
	String() string
	ToURI() string
}

// StringDataSource implements DataSource interface for string-based data sources
type StringDataSource struct {
	value string
	dsType string
}

// NewDataSource creates a new DataSource from a string value
func NewDataSource(value, dsType string) DataSource {
	return &StringDataSource{value: value, dsType: dsType}
}

// String returns the string representation
func (s *StringDataSource) String() string {
	return s.value
}

// ToURI returns the URI representation
func (s *StringDataSource) ToURI() string {
	return s.value
}

// IsDB returns true if this is a database data source
func (s *StringDataSource) IsDB() bool {
	return s.dsType == "db" || strings.Contains(s.value, "postgres") || strings.Contains(s.value, "mysql")
}

// IsCache returns true if this is a cache data source
func (s *StringDataSource) IsCache() bool {
	return s.dsType == "cache" || strings.Contains(s.value, "redis") || strings.Contains(s.value, "valkey")
}

// IsQueue returns true if this is a queue data source
func (s *StringDataSource) IsQueue() bool {
	return s.dsType == "queue" || strings.Contains(s.value, "nats") || strings.Contains(s.value, "amqp")
}

// NewJob creates a new job for framedata
func NewJob(name string, pipe JobResultPipe) interface{} {
	// TODO: Implement proper job creation
	return nil
}

// SubmitJob submits a job for processing in framedata
func SubmitJob(job interface{}) error {
	// TODO: Implement proper job submission
	return nil
}

// ErrorIsNoRows checks if an error is a "no rows" error
func ErrorIsNoRows(err error) bool {
	// TODO: Implement proper error checking
	return false
}
