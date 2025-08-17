package frameconfig

import (
	"context"
	"time"
)

// ConfigManager defines the interface for configuration management
type ConfigManager interface {
	// LoadConfig loads configuration from environment variables
	LoadConfig() error
	
	// LoadConfigWithOIDC loads configuration with OIDC discovery
	LoadConfigWithOIDC(ctx context.Context) error
	
	// GetConfig returns the current configuration
	GetConfig() any
	
	// SetConfig sets the configuration
	SetConfig(config any)
}

// ConfigurationSecurity defines security-related configuration
type ConfigurationSecurity interface {
	IsRunSecurely() bool
}

// ConfigurationLogLevel defines logging configuration
type ConfigurationLogLevel interface {
	LoggingLevel() string
	LoggingFormat() string
	LoggingTimeFormat() string
	LoggingShowStackTrace() bool
	LoggingColored() bool
	LoggingLevelIsDebug() bool
}

// ConfigurationPorts defines port configuration
type ConfigurationPorts interface {
	Port() string
	HTTPPort() string
	GrpcPort() string
}

// ConfigurationWorkerPool defines worker pool configuration
type ConfigurationWorkerPool interface {
	GetCPUFactor() int
	GetCapacity() int
	GetCount() int
	GetExpiryDuration() time.Duration
}

// ConfigurationCORS defines CORS configuration
type ConfigurationCORS interface {
	IsCORSEnabled() bool
	IsCORSAllowCredentials() bool
	GetCORSAllowedHeaders() []string
	GetCORSExposedHeaders() []string
	GetCORSAllowedOrigins() []string
	GetCORSAllowedMethods() []string
	GetCORSMaxAge() int
}

// ConfigurationOAUTH2 defines OAuth2/OIDC configuration
type ConfigurationOAUTH2 interface {
	LoadOauth2Config(ctx context.Context) error
	GetOauth2WellKnownOIDC() string
	GetOauth2WellKnownJwk() string
	GetOauth2WellKnownJwkData() string
	GetOauth2Issuer() string
	GetOauth2AuthorizationEndpoint() string
	GetOauth2RegistrationEndpoint() string
	GetOauth2TokenEndpoint() string
	GetOauth2UserInfoEndpoint() string
	GetOauth2RevocationEndpoint() string
	GetOauth2EndSessionEndpoint() string
	GetOauth2ServiceURI() string
	GetOauth2ServiceClientID() string
	GetOauth2ServiceClientSecret() string
	GetOauth2ServiceAudience() string
	GetOauth2ServiceAdminURI() string
}

// ConfigurationAuthorization defines authorization service configuration
type ConfigurationAuthorization interface {
	GetAuthorizationServiceReadURI() string
	GetAuthorizationServiceWriteURI() string
}

// ConfigurationDatabase defines database configuration
type ConfigurationDatabase interface {
	GetDatabasePrimaryHostURL() []string
	GetDatabaseReplicaHostURL() []string
	DoDatabaseMigrate() bool
	SkipDefaultTransaction() bool
	PreferSimpleProtocol() bool
	GetMaxIdleConnections() int
	GetMaxOpenConnections() int
	GetMaxConnectionLifeTimeInSeconds() time.Duration
	GetDatabaseMigrationPath() string
	CanDatabaseTraceQueries() bool
	GetDatabaseSlowQueryLogThreshold() time.Duration
}

// ConfigurationEvents defines event queue configuration
type ConfigurationEvents interface {
	GetEventsQueueName() string
	GetEventsQueueURL() string
}

// ConfigurationTLS defines TLS configuration
type ConfigurationTLS interface {
	TLSCertPath() string
	TLSCertKeyPath() string
	SetTLSCertAndKeyPath(certificatePath, certificateKeyPath string)
}

// ServiceConfig defines basic service configuration
type ServiceConfig interface {
	ServiceName() string
	ServiceVersion() string
	ServiceEnvironment() string
}
