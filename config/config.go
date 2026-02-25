package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/pitabwire/util"
)

type contextKey string

func (c contextKey) String() string {
	return "frame/config/" + string(c)
}

const (
	ctxKeyConfiguration = contextKey("configurationKey")
	httpStatusOKClass   = 2
	// DefaultSlowQueryThresholdMilliseconds is defined in datastore_logger.go.

	DefaultSlowQueryThreshold = 200 * time.Millisecond
)

// ToContext adds service configuration to the current supplied context.
func ToContext(ctx context.Context, config any) context.Context {
	return context.WithValue(ctx, ctxKeyConfiguration, config)
}

// FromContext extracts service configuration from the supplied context if any exist.
func FromContext[T any](ctx context.Context) T {
	if cfg, ok := ctx.Value(ctxKeyConfiguration).(T); ok {
		return cfg
	}
	var zero T
	return zero
}

// LoadWithOIDC convenience method to process configs.
func LoadWithOIDC[T any](ctx context.Context) (T, error) {
	var cfg T
	cfg, err := FromEnv[T]()
	if err != nil {
		return cfg, err
	}

	oauth2Cfg, ok := any(&cfg).(ConfigurationOAUTH2)
	if ok {
		if oauth2Cfg.GetOauth2ServiceURI() == "" {
			return cfg, nil
		}

		err = oauth2Cfg.LoadOauth2Config(ctx)
		if err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}

// FromEnv convenience method to process configs.
func FromEnv[T any]() (T, error) {
	return env.ParseAs[T]()
}

// FillEnv convenience method to fill a config object with environment data.
func FillEnv(v any) error {
	return env.Parse(v)
}

type ConfigurationDefault struct {
	LogLevel      string `envDefault:"info"                      env:"LOG_LEVEL"       yaml:"log_level"`
	LogFormat     string `envDefault:"info"                      env:"LOG_FORMAT"      yaml:"log_format"`
	LogTimeFormat string `envDefault:"2006-01-02T15:04:05Z07:00" env:"LOG_TIME_FORMAT" yaml:"log_time_format"`
	LogColored    bool   `envDefault:"true"                      env:"LOG_COLORED"     yaml:"log_colored"`

	LogShowStackTrace bool `envDefault:"false" env:"LOG_SHOW_STACK_TRACE" yaml:"log_show_stack_trace"`

	TraceRequests        bool `envDefault:"false" env:"TRACE_REQUESTS"          yaml:"trace_requests"`
	TraceRequestsLogBody bool `envDefault:"false" env:"TRACE_REQUESTS_LOG_BODY" yaml:"trace_requests_log_body"`

	ProfilerEnable   bool   `envDefault:"false" env:"PROFILER_ENABLE" yaml:"profiler_enable"`
	ProfilerPortAddr string `envDefault:":6060" env:"PROFILER_PORT"   yaml:"profiler_port"`

	OpenTelemetryDisable    bool    `envDefault:"false" env:"OPENTELEMETRY_DISABLE"        yaml:"opentelemetry_disable"`
	OpenTelemetryTraceRatio float64 `envDefault:"0.1"   env:"OPENTELEMETRY_TRACE_ID_RATIO" yaml:"opentelemetry_trace_id_ratio"`

	ServiceName        string `envDefault:""     env:"SERVICE_NAME"         yaml:"service_name"`
	ServiceEnvironment string `envDefault:""     env:"SERVICE_ENVIRONMENT"  yaml:"service_environment"`
	ServiceVersion     string `envDefault:""     env:"SERVICE_VERSION"      yaml:"service_version"`
	RunServiceSecurely bool   `envDefault:"true" env:"RUN_SERVICE_SECURELY" yaml:"run_service_securely"`

	RuntimeModeValue  string `envDefault:"polylith" env:"FRAME_RUNTIME_MODE"  yaml:"runtime_mode"`
	ServiceIDValue    string `envDefault:""         env:"FRAME_SERVICE_ID"    yaml:"service_id"`
	ServiceGroupValue string `envDefault:""         env:"FRAME_SERVICE_GROUP" yaml:"service_group"`

	ServerPort     string `envDefault:":7000"  env:"PORT"      yaml:"server_port"`
	HTTPServerPort string `envDefault:":8080"  env:"HTTP_PORT" yaml:"http_server_port"`
	GrpcServerPort string `envDefault:":50051" env:"GRPC_PORT" yaml:"grpc_server_port"`

	// Worker pool settings
	WorkerPoolCPUFactorForWorkerCount int    `envDefault:"10"  env:"WORKER_POOL_CPU_FACTOR_FOR_WORKER_COUNT" yaml:"worker_pool_cpu_factor_for_worker_count"`
	WorkerPoolCapacity                int    `envDefault:"100" env:"WORKER_POOL_CAPACITY"                    yaml:"worker_pool_capacity"`
	WorkerPoolCount                   int    `envDefault:"100" env:"WORKER_POOL_COUNT"                       yaml:"worker_pool_count"`
	WorkerPoolExpiryDuration          string `envDefault:"1s"  env:"WORKER_POOL_EXPIRY_DURATION"             yaml:"worker_pool_expiry_duration"`

	TLSCertificatePath    string `env:"TLS_CERTIFICATE_PATH"     yaml:"tls_certificate_path"`
	TLSCertificateKeyPath string `env:"TLS_CERTIFICATE_KEY_PATH" yaml:"tls_certificate_key_path"`

	Oauth2ServiceURI          string   `env:"OAUTH2_SERVICE_URI"           yaml:"oauth2_service_uri"`
	Oauth2ServiceAdminURI     string   `env:"OAUTH2_SERVICE_ADMIN_URI"     yaml:"oauth2_service_admin_uri"`
	Oauth2WellKnownOIDCPath   string   `env:"OAUTH2_WELL_KNOWN_OIDC_PATH"  yaml:"oauth2_well_known_oidc_path"  envDefault:".well-known/openid-configuration"`
	Oauth2ServiceAudience     []string `env:"OAUTH2_SERVICE_AUDIENCE"      yaml:"oauth2_service_audience"`
	Oauth2ServiceClientID     string   `env:"OAUTH2_SERVICE_CLIENT_ID"     yaml:"oauth2_service_client_id"`
	Oauth2ServiceClientSecret string   `env:"OAUTH2_SERVICE_CLIENT_SECRET" yaml:"oauth2_service_client_secret"`

	Oauth2WellKnownJwkData  string   `env:"OAUTH2_WELL_KNOWN_JWK_DATA" yaml:"oauth2_well_known_jwk_data"`
	Oauth2JwtVerifyAudience []string `env:"OAUTH2_JWT_VERIFY_AUDIENCE" yaml:"oauth2_jwt_verify_audience"`
	Oauth2JwtVerifyIssuer   string   `env:"OAUTH2_JWT_VERIFY_ISSUER"   yaml:"oauth2_jwt_verify_issuer"`

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

	oidcMap OIDCMap `env:"-" yaml:"-"`
}

type ConfigurationService interface {
	Name() string
	Environment() string
	Version() string
}

var _ ConfigurationService = new(ConfigurationDefault)

func (c *ConfigurationDefault) Name() string {
	return c.ServiceName
}
func (c *ConfigurationDefault) Environment() string {
	return c.ServiceEnvironment
}
func (c *ConfigurationDefault) Version() string {
	return c.ServiceVersion
}

type ConfigurationSecurity interface {
	IsRunSecurely() bool
}

var _ ConfigurationSecurity = new(ConfigurationDefault)

func (c *ConfigurationDefault) IsRunSecurely() bool {
	return c.RunServiceSecurely
}

type ConfigurationRuntime interface {
	RuntimeMode() string
	ServiceID() string
	ServiceGroup() string
}

var _ ConfigurationRuntime = new(ConfigurationDefault)

func (c *ConfigurationDefault) RuntimeMode() string {
	return c.RuntimeModeValue
}

func (c *ConfigurationDefault) ServiceID() string {
	return c.ServiceIDValue
}

func (c *ConfigurationDefault) ServiceGroup() string {
	return c.ServiceGroupValue
}

type ConfigurationLogLevel interface {
	LoggingLevel() string
	LoggingFormat() string
	LoggingTimeFormat() string
	LoggingShowStackTrace() bool
	LoggingColored() bool
	LoggingLevelIsDebug() bool
}

var _ ConfigurationLogLevel = new(ConfigurationDefault)

func (c *ConfigurationDefault) LoggingLevel() string {
	return c.LogLevel
}

func (c *ConfigurationDefault) LoggingTimeFormat() string {
	return c.LogTimeFormat
}

func (c *ConfigurationDefault) LoggingFormat() string {
	return c.LogFormat
}

func (c *ConfigurationDefault) LoggingColored() bool {
	return c.LogColored
}

func (c *ConfigurationDefault) LoggingShowStackTrace() bool {
	return c.LogShowStackTrace
}

func (c *ConfigurationDefault) LoggingLevelIsDebug() bool {
	return c.LoggingLevel() == "debug" || c.LoggingLevel() == "trace"
}

type ConfigurationTraceRequests interface {
	TraceReq() bool
	TraceReqLogBody() bool
}

var _ ConfigurationTraceRequests = new(ConfigurationDefault)

func (c *ConfigurationDefault) TraceReq() bool {
	return c.TraceRequests
}

func (c *ConfigurationDefault) TraceReqLogBody() bool {
	return c.TraceRequestsLogBody
}

type ConfigurationProfiler interface {
	ProfilerEnabled() bool
	ProfilerPort() string
}

var _ ConfigurationProfiler = new(ConfigurationDefault)

func (c *ConfigurationDefault) ProfilerEnabled() bool {
	return c.ProfilerEnable
}

func (c *ConfigurationDefault) ProfilerPort() string {
	if c.ProfilerPortAddr != "" {
		return c.ProfilerPortAddr
	}
	return ":6060"
}

type ConfigurationPorts interface {
	Port() string
	HTTPPort() string
	GrpcPort() string
}

var _ ConfigurationPorts = new(ConfigurationDefault)

func (c *ConfigurationDefault) Port() string {
	if i, err := strconv.Atoi(c.ServerPort); err == nil && i > 0 {
		return fmt.Sprintf(":%s", strings.TrimSpace(c.ServerPort))
	}

	if strings.HasPrefix(c.ServerPort, ":") || strings.Contains(c.ServerPort, ":") {
		return c.ServerPort
	}

	return ":80"
}

func (c *ConfigurationDefault) HTTPPort() string {
	if i, err := strconv.Atoi(c.HTTPServerPort); err == nil && i > 0 {
		return fmt.Sprintf(":%s", strings.TrimSpace(c.HTTPServerPort))
	}

	if strings.HasPrefix(c.HTTPServerPort, ":") || strings.Contains(c.HTTPServerPort, ":") {
		return c.HTTPServerPort
	}

	return ":8080"
}

func (c *ConfigurationDefault) GrpcPort() string {
	if i, err := strconv.Atoi(c.GrpcServerPort); err == nil && i > 0 {
		return fmt.Sprintf(":%s", strings.TrimSpace(c.GrpcServerPort))
	}

	if strings.HasPrefix(c.GrpcServerPort, ":") || strings.Contains(c.GrpcServerPort, ":") {
		return c.GrpcServerPort
	}

	return c.Port()
}

type ConfigurationTelemetry interface {
	DisableOpenTelemetry() bool
	SamplingRatio() float64
}

var _ ConfigurationTelemetry = new(ConfigurationDefault)

func (c *ConfigurationDefault) DisableOpenTelemetry() bool {
	return c.OpenTelemetryDisable
}

func (c *ConfigurationDefault) SamplingRatio() float64 {
	return c.OpenTelemetryTraceRatio
}

type ConfigurationWorkerPool interface {
	GetCPUFactor() int
	GetCapacity() int
	GetCount() int
	GetExpiryDuration() time.Duration
}

var _ ConfigurationWorkerPool = new(ConfigurationDefault)

func (c *ConfigurationDefault) GetCPUFactor() int {
	return c.WorkerPoolCPUFactorForWorkerCount
}

func (c *ConfigurationDefault) GetCapacity() int {
	return c.WorkerPoolCapacity
}

func (c *ConfigurationDefault) GetCount() int {
	return c.WorkerPoolCount
}

func (c *ConfigurationDefault) GetExpiryDuration() time.Duration {
	if c.WorkerPoolExpiryDuration != "" {
		duration, err := time.ParseDuration(c.WorkerPoolExpiryDuration)
		if err == nil {
			return duration
		}
	}

	return time.Second
}

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
	GetOauth2ServiceAudience() []string
	GetOauth2ServiceAdminURI() string
}

var _ ConfigurationOAUTH2 = new(ConfigurationDefault)

func (c *ConfigurationDefault) LoadOauth2Config(ctx context.Context) error {
	if len(c.oidcMap) == 0 {
		c.oidcMap = make(map[string]any)
	}

	err := c.oidcMap.loadOIDC(ctx, c.GetOauth2WellKnownOIDC())
	if err != nil {
		return err
	}

	c.Oauth2WellKnownJwkData, err = c.oidcMap.loadJWKData(ctx, c.GetOauth2WellKnownJwk())
	if err != nil {
		return err
	}
	return nil
}
func (c *ConfigurationDefault) GetOauth2WellKnownOIDC() string {
	res, _ := url.JoinPath(c.GetOauth2ServiceURI(), c.Oauth2WellKnownOIDCPath)
	return res
}
func (c *ConfigurationDefault) GetOauth2WellKnownJwk() string {
	val, ok := c.oidcMap["jwks_uri"]
	if !ok {
		return ""
	}
	sVal, typeOk := val.(string)
	if !typeOk {
		// Optionally log an error here if the type is unexpectedly different
		// c.Log(ctx).Warnf("OIDC map value for 'jwks_uri' is not a string: %T", val)
		return ""
	}
	return sVal
}

func (c *ConfigurationDefault) GetOauth2WellKnownJwkData() string {
	return c.Oauth2WellKnownJwkData
}
func (c *ConfigurationDefault) GetOauth2Issuer() string {
	val, ok := c.oidcMap["issuer"]
	if !ok {
		return ""
	}
	sVal, ok := val.(string)
	if !ok {
		return ""
	}
	return sVal
}
func (c *ConfigurationDefault) GetOauth2AuthorizationEndpoint() string {
	val, ok := c.oidcMap["authorization_endpoint"]
	if !ok {
		return ""
	}
	sVal, ok := val.(string)
	if !ok {
		return ""
	}
	return sVal
}
func (c *ConfigurationDefault) GetOauth2RegistrationEndpoint() string {
	val, ok := c.oidcMap["registration_endpoint"]
	if !ok {
		return ""
	}
	sVal, ok := val.(string)
	if !ok {
		return ""
	}
	return sVal
}
func (c *ConfigurationDefault) GetOauth2TokenEndpoint() string {
	val, ok := c.oidcMap["token_endpoint"]
	if !ok {
		return ""
	}
	sVal, ok := val.(string)
	if !ok {
		return ""
	}
	return sVal
}
func (c *ConfigurationDefault) GetOauth2UserInfoEndpoint() string {
	val, ok := c.oidcMap["userinfo_endpoint"]
	if !ok {
		return ""
	}
	sVal, ok := val.(string)
	if !ok {
		return ""
	}
	return sVal
}
func (c *ConfigurationDefault) GetOauth2RevocationEndpoint() string {
	val, ok := c.oidcMap["revocation_endpoint"]
	if !ok {
		return ""
	}
	sVal, ok := val.(string)
	if !ok {
		return ""
	}
	return sVal
}
func (c *ConfigurationDefault) GetOauth2EndSessionEndpoint() string {
	val, ok := c.oidcMap["end_session_endpoint"]
	if !ok {
		return ""
	}
	sVal, ok := val.(string)
	if !ok {
		return ""
	}
	return sVal
}

func (c *ConfigurationDefault) GetOauth2ServiceURI() string {
	return c.Oauth2ServiceURI
}

func (c *ConfigurationDefault) GetOauth2ServiceClientID() string {
	return c.Oauth2ServiceClientID
}
func (c *ConfigurationDefault) GetOauth2ServiceClientSecret() string {
	return c.Oauth2ServiceClientSecret
}
func (c *ConfigurationDefault) GetOauth2ServiceAudience() []string {
	return c.Oauth2ServiceAudience
}
func (c *ConfigurationDefault) GetOauth2ServiceAdminURI() string {
	return c.Oauth2ServiceAdminURI
}

type ConfigurationJWTVerification interface {
	GetOauth2WellKnownJwk() string
	GetVerificationAudience() []string
	GetVerificationIssuer() string
}

var _ ConfigurationJWTVerification = new(ConfigurationDefault)

func (c *ConfigurationDefault) GetVerificationAudience() []string {
	return c.Oauth2JwtVerifyAudience
}
func (c *ConfigurationDefault) GetVerificationIssuer() string {
	return c.Oauth2JwtVerifyIssuer
}

type ConfigurationAuthorization interface {
	GetAuthorizationServiceReadURI() string
	GetAuthorizationServiceWriteURI() string
	AuthorizationServiceCanRead() bool
	AuthorizationServiceCanWrite() bool
}

var _ ConfigurationAuthorization = new(ConfigurationDefault)

func (c *ConfigurationDefault) GetAuthorizationServiceReadURI() string {
	return c.AuthorizationServiceReadURI
}

func (c *ConfigurationDefault) GetAuthorizationServiceWriteURI() string {
	return c.AuthorizationServiceWriteURI
}
func (c *ConfigurationDefault) AuthorizationServiceCanRead() bool {
	return c.AuthorizationServiceReadURI != ""
}

func (c *ConfigurationDefault) AuthorizationServiceCanWrite() bool {
	return c.AuthorizationServiceWriteURI != ""
}

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
}

type ConfigurationDatabaseTracing interface {
	CanDatabaseTraceQueries() bool
	GetDatabaseSlowQueryLogThreshold() time.Duration
}

var _ ConfigurationDatabase = new(ConfigurationDefault)

func (c *ConfigurationDefault) GetDatabasePrimaryHostURL() []string {
	return c.DatabasePrimaryURL
}

func (c *ConfigurationDefault) GetDatabaseReplicaHostURL() []string {
	return c.DatabaseReplicaURL
}

func (c *ConfigurationDefault) DoDatabaseMigrate() bool {
	stdArgs := os.Args[1:]
	return c.DatabaseMigrate || (len(stdArgs) > 0 && stdArgs[0] == "migrate")
}

func (c *ConfigurationDefault) PreferSimpleProtocol() bool {
	return c.DatabasePreferSimpleProtocol
}

func (c *ConfigurationDefault) SkipDefaultTransaction() bool {
	return c.DatabaseSkipDefaultTransaction
}

func (c *ConfigurationDefault) GetMaxIdleConnections() int {
	return c.DatabaseMaxIdleConnections
}

func (c *ConfigurationDefault) GetMaxOpenConnections() int {
	return c.DatabaseMaxOpenConnections
}

func (c *ConfigurationDefault) GetMaxConnectionLifeTimeInSeconds() time.Duration {
	return time.Duration(c.DatabaseMaxConnectionLifeTimeSeconds) * time.Second
}

func (c *ConfigurationDefault) GetDatabaseMigrationPath() string {
	return c.DatabaseMigrationPath
}

var _ ConfigurationDatabaseTracing = new(ConfigurationDefault)

func (c *ConfigurationDefault) CanDatabaseTraceQueries() bool {
	return c.DatabaseTraceQueries
}
func (c *ConfigurationDefault) GetDatabaseSlowQueryLogThreshold() time.Duration {
	threshold, err := time.ParseDuration(c.DatabaseSlowQueryLogThreshold)
	if err != nil {
		threshold = DefaultSlowQueryThreshold
	}
	return threshold
}

type ConfigurationEvents interface {
	GetEventsQueueName() string
	GetEventsQueueURL() string
}

var _ ConfigurationEvents = new(ConfigurationDefault)

func (c *ConfigurationDefault) GetEventsQueueName() string {
	if strings.TrimSpace(c.EventsQueueName) == "" {
		return "frame.events.internal_._queue"
	}

	return c.EventsQueueName
}

func (c *ConfigurationDefault) GetEventsQueueURL() string {
	if strings.TrimSpace(c.EventsQueueURL) == "" {
		return "mem://frame.events.internal_._queue"
	}

	return c.EventsQueueURL
}

type ConfigurationTLS interface {
	TLSCertPath() string
	TLSCertKeyPath() string
	SetTLSCertAndKeyPath(certificatePath, certificateKeyPath string)
}

var _ ConfigurationTLS = new(ConfigurationDefault)

func (c *ConfigurationDefault) TLSCertKeyPath() string {
	return c.TLSCertificateKeyPath
}
func (c *ConfigurationDefault) TLSCertPath() string {
	return c.TLSCertificatePath
}

func (c *ConfigurationDefault) SetTLSCertAndKeyPath(certificatePath, certificateKeyPath string) {
	c.TLSCertificatePath = certificatePath
	c.TLSCertificateKeyPath = certificateKeyPath
}

type OIDCMap map[string]any

func (oid *OIDCMap) loadOIDC(ctx context.Context, url string) error {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	hreq.Header.Set("Accept", "application/jrd+json,application/json;q=0.9")

	//nolint:bodyclose // closed by util.CloseAndLogOnError below
	hresp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return err
	}
	defer util.CloseAndLogOnError(ctx, hresp.Body)

	if hresp.StatusCode/100 != httpStatusOKClass {
		return fmt.Errorf("OIDC discovery request %q failed: %d %s", url, hresp.StatusCode, hresp.Status)
	}

	err = json.NewDecoder(hresp.Body).Decode(oid)
	if err != nil {
		return fmt.Errorf("decoding OIDC discovery response from %q: %w", url, err)
	}

	return nil
}

type Jwks struct {
	Keys []JSONWebKeys `json:"keys"`
}

type JSONWebKeys struct {
	Kty string   `json:"kty"`
	Kid string   `json:"kid"`
	Use string   `json:"use"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5c []string `json:"x5c"`
}

func (oid *OIDCMap) loadJWKData(ctx context.Context, url string) (string, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	hreq.Header.Set("Accept", "application/jrd+json,application/json;q=0.9")

	//nolint:bodyclose // closed by util.CloseAndLogOnError below
	hresp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return "", err
	}
	defer util.CloseAndLogOnError(ctx, hresp.Body)

	if hresp.StatusCode/100 != httpStatusOKClass {
		return "", fmt.Errorf("JWKs data request %q failed: %d %s", url, hresp.StatusCode, hresp.Status)
	}

	var jwkData []byte
	jwkData, err = io.ReadAll(hresp.Body)
	jwkString := string(jwkData)

	return jwkString, err
}
