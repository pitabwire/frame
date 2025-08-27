package frame

import (
	"context"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
)

const (
	ctxKeyConfiguration = contextKey("configurationKey")
	// DefaultSlowQueryThresholdMilliseconds is defined in datastore_logger.go.
	DefaultSlowQueryThreshold = 200 * time.Millisecond // Default slow query threshold
)

// WithConfig Option that helps to specify or override the configuration object of our service.
func WithConfig(config any) Option {
	return func(_ context.Context, s Service) {
		if impl, ok := s.(*serviceImpl); ok {
			impl.configuration = config
		}
	}
}

// ConfigToContext adds service configuration to the current supplied context.
func ConfigToContext(ctx context.Context, config any) context.Context {
	return context.WithValue(ctx, ctxKeyConfiguration, config)
}

// Cfg extracts service configuration from the supplied context if any exist.
func Cfg(ctx context.Context) any {
	return ctx.Value(ctxKeyConfiguration)
}

// ConfigFromEnv convenience method to process configs.
func ConfigFromEnv[T any]() (T, error) {
	return env.ParseAs[T]()
}

// ConfigFillEnv convenience method to fill a config object with environment data.
func ConfigFillEnv(v any) error {
	return env.Parse(v)
}

type Configuration struct {
	LogLevel          string `envDefault:"info"                      env:"LOG_LEVEL"            yaml:"log_level"`
	LogFormat         string `envDefault:"info"                      env:"LOG_FORMAT"           yaml:"log_format"`
	LogTimeFormat     string `envDefault:"2006-01-02T15:04:05Z07:00" env:"LOG_TIME_FORMAT"      yaml:"log_time_format"`
	LogColored        bool   `envDefault:"true"                      env:"LOG_COLORED"          yaml:"log_colored"`
	LogShowStackTrace bool   `envDefault:"false"                     env:"LOG_SHOW_STACK_TRACE" yaml:"log_show_stack_trace"`

	ServiceName        string `envDefault:""     env:"SERVICE_NAME"         yaml:"service_name"`
	ServiceEnvironment string `envDefault:""     env:"SERVICE_ENVIRONMENT"  yaml:"service_environment"`
	ServiceVersion     string `envDefault:""     env:"SERVICE_VERSION"      yaml:"service_version"`
	RunServiceSecurely bool   `envDefault:"true" env:"RUN_SERVICE_SECURELY" yaml:"run_service_securely"`

	ServerPort     string `envDefault:":7000"  env:"PORT"      yaml:"server_port"`
	HTTPServerPort string `envDefault:":8080"  env:"HTTP_PORT" yaml:"http_server_port"`
	GrpcServerPort string `envDefault:":50051" env:"GRPC_PORT" yaml:"grpc_server_port"`

	// Worker pool settings
	WorkerPoolCPUFactorForWorkerCount int    `envDefault:"10"  env:"WORKER_POOL_CPU_FACTOR_FOR_WORKER_COUNT" yaml:"worker_pool_cpu_factor_for_worker_count"`
	WorkerPoolCapacity                int    `envDefault:"100" env:"WORKER_POOL_CAPACITY"                    yaml:"worker_pool_capacity"`
	WorkerPoolCount                   int    `envDefault:"100" env:"WORKER_POOL_COUNT"                       yaml:"worker_pool_count"`
	WorkerPoolExpiryDuration          string `envDefault:"1s"  env:"WORKER_POOL_EXPIRY_DURATION"             yaml:"worker_pool_expiry_duration"`

	CORSEnabled          bool     `envDefault:"false"                     env:"CORS_ENABLED"           yaml:"cors_enabled"`
	CORSAllowCredentials bool     `envDefault:"false"                     env:"CORS_ALLOW_CREDENTIALS" yaml:"cors_allow_credentials"`
	CORSAllowedHeaders   []string `envDefault:"Authorization"             env:"CORS_ALLOWED_HEADERS"   yaml:"cors_allowed_headers"`
	CORSExposedHeaders   []string `envDefault:"*"                         env:"CORS_EXPOSED_HEADERS"   yaml:"cors_exposed_headers"`
	CORSAllowedOrigins   []string `envDefault:"*"                         env:"CORS_ALLOWED_ORIGINS"   yaml:"cors_allowed_origins"`
	CORSAllowedMethods   []string `envDefault:"GET,HEAD,POST,PUT,OPTIONS" env:"CORS_ALLOWED_METHODS"   yaml:"cors_allowed_methods"`
	CORSMaxAge           int      `envDefault:"3600"                      env:"CORS_MAX_AGE"           yaml:"cors_max_age"`

	TLSCertificatePath    string `env:"TLS_CERTIFICATE_PATH"     yaml:"tls_certificate_path"`
	TLSCertificateKeyPath string `env:"TLS_CERTIFICATE_KEY_PATH" yaml:"tls_certificate_key_path"`

	Oauth2ServiceURI          string `env:"OAUTH2_SERVICE_URI"           yaml:"oauth2_service_uri"`
	Oauth2ServiceAdminURI     string `env:"OAUTH2_SERVICE_ADMIN_URI"     yaml:"oauth2_service_admin_uri"`
	Oauth2WellKnownOIDCPath   string `env:"OAUTH2_WELL_KNOWN_OIDC_PATH"  yaml:"oauth2_well_known_oidc_path"  envDefault:".well-known/openid-configuration"`
	Oauth2WellKnownJwkData    string `env:"OAUTH2_WELL_KNOWN_JWK_DATA"   yaml:"oauth2_well_known_jwk_data"`
	Oauth2ServiceAudience     string `env:"OAUTH2_SERVICE_AUDIENCE"      yaml:"oauth2_service_audience"`
	Oauth2JwtVerifyAudience   string `env:"OAUTH2_JWT_VERIFY_AUDIENCE"   yaml:"oauth2_jwt_verify_audience"`
	Oauth2JwtVerifyIssuer     string `env:"OAUTH2_JWT_VERIFY_ISSUER"     yaml:"oauth2_jwt_verify_issuer"`
	Oauth2ServiceClientID     string `env:"OAUTH2_SERVICE_CLIENT_ID"     yaml:"oauth2_service_client_id"`
	Oauth2ServiceClientSecret string `env:"OAUTH2_SERVICE_CLIENT_SECRET" yaml:"oauth2_service_client_secret"`

	AuthorizationServiceReadURI  string `env:"AUTHORIZATION_SERVICE_READ_URI"  yaml:"authorization_service_read_uri"`
	AuthorizationServiceWriteURI string `env:"AUTHORIZATION_SERVICE_WRITE_URI" yaml:"authorization_service_write_uri"`

	DatabasePrimaryURL             []string `env:"DATABASE_URL"             yaml:"database_url"`
	DatabaseReplicaURL             []string `env:"REPLICA_DATABASE_URL"     yaml:"replica_database_url"`
	DatabaseMigrate                bool     `env:"DO_MIGRATION"             yaml:"do_migration"             envDefault:"false"`
	DatabaseMigrationPath          string   `env:"MIGRATION_PATH"           yaml:"migration_path"           envDefault:"./migrations/0001"`
	DatabaseSkipDefaultTransaction bool     `env:"SKIP_DEFAULT_TRANSACTION" yaml:"skip_default_transaction" envDefault:"true"`
	DatabasePreferSimpleProtocol   bool     `env:"PREFER_SIMPLE_PROTOCOL"   yaml:"prefer_simple_protocol"   envDefault:"true"`

	DatabaseMaxIdleConnections           int `envDefault:"2"   env:"DATABASE_MAX_IDLE_CONNECTIONS"                yaml:"database_max_idle_connections"`
	DatabaseMaxOpenConnections           int `envDefault:"5"   env:"DATABASE_MAX_OPEN_CONNECTIONS"                yaml:"database_max_open_connections"`
	DatabaseMaxConnectionLifeTimeSeconds int `envDefault:"300" env:"DATABASE_MAX_CONNECTION_LIFE_TIME_IN_SECONDS" yaml:"database_max_connection_life_time_seconds"`

	DatabaseTraceQueries          bool   `envDefault:"false" env:"DATABASE_LOG_QUERIES"          yaml:"database_log_queries"`
	DatabaseSlowQueryLogThreshold string `envDefault:"200ms" env:"DATABASE_SLOW_QUERY_THRESHOLD" yaml:"database_slow_query_threshold"`

	EventsQueueName string `envDefault:"frame.events.internal_._queue"       env:"EVENTS_QUEUE_NAME" yaml:"events_queue_name"`
	EventsQueueURL  string `envDefault:"mem://frame.events.internal_._queue" env:"EVENTS_QUEUE_URL"  yaml:"events_queue_url"`
}