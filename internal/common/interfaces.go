package common

import "time"

import (
	"context"
	"net/http"
	"net/url"

	"github.com/pitabwire/util"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// Logger interface for logging functionality
type Logger interface {
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger
	Fatal(args ...interface{})
	Error(args ...interface{})
	Warn(args ...interface{})
	Info(args ...interface{})
	Debug(args ...interface{})
}

// ServiceInterface defines the minimal interface needed across modules
type ServiceInterface interface {
	Config() interface{}
	GetModule(moduleType string) interface{}
	InvokeRestService(ctx context.Context, method string, url string, payload map[string]any, headers map[string][]string) (int, []byte, error)
	InvokeRestServiceURLEncoded(ctx context.Context, method string, url string, payload url.Values, headers map[string]string) (int, []byte, error)
	Log(ctx context.Context) *util.LogEntry
	Name() string
	RegisterModule(module Module)
	AddCleanupMethod(func(context.Context))
	HandleHealth(http.ResponseWriter, *http.Request)
	DB(ctx context.Context, readOnly bool) *gorm.DB
	DBPool(name ...string) interface{}
	DBWithName(ctx context.Context, name string, readOnly bool) *gorm.DB
	Stop(ctx context.Context)
}

// Service defines the service interface for type assertions
type Service interface {
	ServiceInterface
	HandleHealth(http.ResponseWriter, *http.Request)
	DB(ctx context.Context, readOnly bool) *gorm.DB
	DBPool(name ...string) interface{}
	DBWithName(ctx context.Context, name string, readOnly bool) *gorm.DB
}

// ConfigurationSecurity defines the security configuration interface
type ConfigurationSecurity interface {
	IsRunSecurely() bool
}

// ConfigurationTLS defines the TLS configuration interface
type ConfigurationTLS interface {
	GetTLSCertificateFile() string
	GetTLSPrivateKeyFile() string
	TLSCertPath() string
	TLSCertKeyPath() string
}

// ConfigurationDatabase defines the database configuration interface
type ConfigurationDatabase interface {
	GetDatabaseURL() string
	GetDatabasePrimaryHostURL() []string
	GetDatabaseReplicaHostURL() []string
	DoDatabaseMigrate() bool
	GetDatabaseSlowQueryLogThreshold() time.Duration
	CanDatabaseTraceQueries() bool
	PreferSimpleProtocol() bool
	SkipDefaultTransaction() bool
	GetMaxOpenConnections() int
	GetMaxIdleConnections() int
	GetMaxConnectionLifeTimeInSeconds() time.Duration
}

// ConfigurationOAUTH2 interface for OAuth2 configuration
type ConfigurationOAUTH2 interface {
	GetOauth2WellKnownJwkData() string
	GetOauth2ServiceAdminURI() string
	GetOauth2ServiceClientID() string
	GetOauth2ServiceClientSecret() string
	GetOauth2ServiceAudience() string
}

// ConfigurationLogLevel defines the log level configuration interface
type ConfigurationLogLevel interface {
	GetLogLevel() string
	LoggingTimeFormat() string
	LoggingColored() bool
}

// Option defines the service option function type
type Option func(context.Context, Service)

// ContextKey defines a type for context keys to avoid collisions
type ContextKey string

func (c ContextKey) String() string {
	return "frame/" + string(c)
}

// ServerStreamWrapper simple wrapper method that stores auth claims for the server stream context
type ServerStreamWrapper struct {
	Ctx context.Context
	grpc.ServerStream
}

// Context converts the stream wrappers claims to be contained in the stream context
func (s *ServerStreamWrapper) Context() context.Context {
	return s.Ctx
}

// Module interface defines common module functionality
type Module interface {
	Name() string
	Status() string
	Enabled() bool
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthCheck(ctx context.Context) error
}

// ModuleType constants for module identification
const (
	ModuleTypeAuth           = "auth"
	ModuleTypeAuthorization  = "authorization"
	ModuleTypeData           = "data"
	ModuleTypeQueue          = "queue"
	ModuleTypeServer         = "server"
	ModuleTypeLocalization   = "localization"
	ModuleTypeWorker         = "worker"
	ModuleTypeWorkerPool     = "workerPool"
	ModuleTypeObservability  = "observability"
	ModuleTypeLogging        = "logging"
	ModuleTypeHealth         = "health"
)

// Common function signatures for context operations
type ClaimsFromContextFunc func(ctx context.Context) interface{}
type LanguageFromContextFunc func(ctx context.Context) string
type LanguageToMapFunc func(metadata map[string]string, language string) map[string]string

// HTTP Handler type for middleware
type HTTPHandler http.Handler

// Svc is a global variable reference for backward compatibility
var Svc ServiceInterface

// ConfigurationAuthorization defines the authorization configuration interface
type ConfigurationAuthorization interface {
	GetAuthorizationURL() string
	GetAuthorizationServiceReadURI() string
}

// ConfigurationLogLevel interface already defined above

// ResponseWriter defines the HTTP response writer interface
type ResponseWriter interface {
	http.ResponseWriter
}

// WorkerPoolModule interface for worker pool module
type WorkerPoolModule interface {
	Module
	Pool() interface{}
	PoolOptions() interface{}
}

// QueueManager interface for queue management
type QueueManager interface {
	AddPublisher(ctx context.Context, reference string, queueURL string) error
	DiscardPublisher(ctx context.Context, reference string) error
	GetPublisher(reference string) (interface{}, error)
	Publish(ctx context.Context, reference string, payload interface{}) error
}

// QueueModule interface for queue module
type QueueModule interface {
	Module
	QueueManager() QueueManager
	EventRegistry() map[string]interface{}
	Queue() interface{} // Returns the underlying queue implementation
}

// ServerModule interface for server module functionality
type ServerModule interface {
	Module
	ServerManager() ServerManager
}

// DataModule interface for data module
type DataModule interface {
	Module
	DataStores() map[string]interface{}
}

// HealthModule interface for health module
type HealthModule interface {
	Module
	HealthCheck(ctx context.Context) error
	HealthCheckers() []interface{}
}

// AuthenticationClaims interface for authentication claims
type AuthenticationClaims interface {
	GetTenantID() string
	GetPartitionID() string
	GetAccessID() string
	AsMetadata() map[string]string
}

// ClaimsFromContext extracts authentication claims from context
func ClaimsFromContext(ctx context.Context) AuthenticationClaims {
	// This is a placeholder - actual implementation will be in frameauth module
	// using a context key to extract claims
	return nil
}

// IsTenancyChecksOnClaimSkipped checks if tenancy checks should be skipped
func IsTenancyChecksOnClaimSkipped(ctx context.Context) bool {
	// This is a placeholder - actual implementation will be in frameauth module
	return false
}

// LanguageFromContext extracts language from context
func LanguageFromContext(ctx context.Context) string {
	// This will be implemented by the framelocalization module
	return ""
}

// LanguageToMap adds language to metadata map
func LanguageToMap(metadata map[string]string, language string) map[string]string {
	// This will be implemented by the framelocalization module
	return metadata
}

// NewLoggingModule creates a new logging module - placeholder for framelogging
func NewLoggingModule(logger interface{}) Module {
	// This will be implemented by the framelogging module
	return nil
}

// NewObservabilityModule creates a new observability module - placeholder for frameobservability
func NewObservabilityModule(manager interface{}, enableTracing interface{}, textMap interface{}, exporter interface{}, sampler interface{}, reader interface{}, logsExporter interface{}) Module {
	// This will be implemented by the frameobservability module
	return nil
}

// NewLocalizationModule creates a new localization module - placeholder for framelocalization
func NewLocalizationModule(bundle interface{}) Module {
	// This will be implemented by the framelocalization module
	return nil
}

// NewServerManager creates a new server manager - placeholder for frameserver
func NewServerManager(config interface{}, logger interface{}, metricsCollector interface{}) ServerManager {
	// This will be implemented by the frameserver module
	return nil
}

// NewServerModule creates a new server module - placeholder for frameserver
func NewServerModule(manager interface{}, config interface{}) ServerModule {
	// This will be implemented by the frameserver module
	return nil
}

// ServerStats interface for server statistics
type ServerStats interface {
	GetConnectionCount() int
	GetRequestCount() int64
	GetActiveConnections() int64
	GetTotalRequests() int64
}

// ServerManager interface for server management
type ServerManager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetHTTPAddress() string
	GetGRPCAddress() string
	GetGRPCServer() *grpc.Server
	GetHTTPServer() *http.Server
	GetServerStats() ServerStats
}

// NewWorkerPoolModule creates a new worker pool module - placeholder for frameworker
func NewWorkerPoolModule(pool interface{}, options interface{}, client interface{}) WorkerPoolModule {
	// This will be implemented by the frameworker module
	return nil
}

// NewDatastoreManager creates a new datastore manager - placeholder for framedata
func NewDatastoreManager(config interface{}, logger interface{}, metrics interface{}) interface{} {
	// This will be implemented by the framedata module
	return nil
}

// NewMigrator creates a new migrator - placeholder for framedata
func NewMigrator(datastoreManager interface{}, config interface{}, logger interface{}, filesystem interface{}) interface{} {
	// This will be implemented by the framedata module
	return nil
}

// NewHealthModule creates a new health module - placeholder for frameserver
func NewHealthModule(config interface{}) HealthModule {
	// This will be implemented by the frameserver module
	return nil
}

// JobResultPipe represents a job result pipe
type JobResultPipe interface{}

// NewJob creates a new job
func NewJob(name string, pipe JobResultPipe) interface{} {
	// TODO: Implement proper job creation
	return nil
}

// SubmitJob submits a job for processing
func SubmitJob(job interface{}) error {
	// TODO: Implement proper job submission
	return nil
}

// DataSource represents a data source configuration
type DataSource interface{}
