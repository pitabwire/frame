package frame

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/caarlos0/env/v11"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const ctxKeyConfiguration = contextKey("configurationKey")

// Config Option that helps to specify or override the configuration object of our service.
func Config(config any) Option {
	return func(s *Service) {
		s.configuration = config
	}
}

func (s *Service) Config() any {
	return s.configuration
}

// ConfigToContext adds service configuration to the current supplied context
func ConfigToContext(ctx context.Context, config any) context.Context {
	return context.WithValue(ctx, ctxKeyConfiguration, config)
}

// Cfg extracts service configuration from the supplied context if any exist
func Cfg(ctx context.Context) any {
	return ctx.Value(ctxKeyConfiguration)
}

// ConfigLoadWithOIDC convenience method to process configs
func ConfigLoadWithOIDC[T any](ctx context.Context) (cfg T, err error) {
	cfg, err = ConfigFromEnv[T]()
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

// ConfigFromEnv convenience method to process configs
func ConfigFromEnv[T any]() (cfg T, err error) {
	return env.ParseAs[T]()
}

// ConfigFillFromEnv convenience method to process configs
func ConfigFillFromEnv(cfg any) error {
	return env.Parse(cfg)
}

type ConfigurationDefault struct {
	LogLevel           string `envDefault:"info" env:"LOG_LEVEL" yaml:"log_level"`
	LogFormat          string `envDefault:"info" env:"LOG_FORMAT" yaml:"log_format"`
	LogTimeFormat      string `envDefault:"2006-01-02T15:04:05Z07:00" env:"LOG_TIME_FORMAT" yaml:"log_time_format"`
	LogColored         bool   `envDefault:"true" env:"LOG_COLORED" yaml:"log_colored"`
	LogShowStackTrace  bool   `envDefault:"false" env:"LOG_SHOW_STACK_TRACE" yaml:"log_show_stack_trace"`
	RunServiceSecurely bool   `envDefault:"true" env:"RUN_SERVICE_SECURELY" yaml:"run_service_securely"`

	ServerPort     string `envDefault:":7000" env:"PORT" yaml:"server_port"`
	HttpServerPort string `envDefault:":8080" env:"HTTP_PORT" yaml:"http_server_port"`
	GrpcServerPort string `envDefault:":50051" env:"GRPC_PORT" yaml:"grpc_server_port"`

	CORSEnabled          bool     `envDefault:"false" env:"CORS_ENABLED" yaml:"cors_enabled"`
	CORSAllowCredentials bool     `envDefault:"false" env:"CORS_ALLOW_CREDENTIALS" yaml:"cors_allow_credentials"`
	CORSAllowedHeaders   []string `envDefault:"Authorization" env:"CORS_ALLOWED_HEADERS" yaml:"cors_allowed_headers"`
	CORSExposedHeaders   []string `envDefault:"*" env:"CORS_EXPOSED_HEADERS" yaml:"cors_exposed_headers"`
	CORSAllowedOrigins   []string `envDefault:"*" env:"CORS_ALLOWED_ORIGINS" yaml:"cors_allowed_origins"`
	CORSAllowedMethods   []string `envDefault:"GET,HEAD,POST,PUT,OPTIONS" env:"CORS_ALLOWED_METHODS" yaml:"cors_allowed_methods"`
	CORSMaxAge           int      `envDefault:"3600" env:"CORS_MAX_AGE" yaml:"cors_max_age"`

	TLSCertificatePath    string `env:"TLS_CERTIFICATE_PATH" yaml:"tls_certificate_path"`
	TLSCertificateKeyPath string `env:"TLS_CERTIFICATE_KEY_PATH" yaml:"tls_certificate_key_path"`

	Oauth2ServiceURI          string `env:"OAUTH2_SERVICE_URI" yaml:"oauth2_service_uri"`
	Oauth2ServiceAdminURI     string `env:"OAUTH2_SERVICE_ADMIN_URI" yaml:"oauth2_service_admin_uri"`
	Oauth2WellKnownOIDCPath   string `envDefault:".well-known/openid-configuration" env:"OAUTH2_WELL_KNOWN_OIDC_PATH" yaml:"oauth2_well_known_oidc_path"`
	Oauth2WellKnownJwkData    string `env:"OAUTH2_WELL_KNOWN_JWK_DATA" yaml:"oauth2_well_known_jwk_data"`
	Oauth2ServiceAudience     string `env:"OAUTH2_SERVICE_AUDIENCE" yaml:"oauth2_service_audience"`
	Oauth2JwtVerifyAudience   string `env:"OAUTH2_JWT_VERIFY_AUDIENCE" yaml:"oauth2_jwt_verify_audience"`
	Oauth2JwtVerifyIssuer     string `env:"OAUTH2_JWT_VERIFY_ISSUER" yaml:"oauth2_jwt_verify_issuer"`
	Oauth2ServiceClientSecret string `env:"OAUTH2_SERVICE_CLIENT_SECRET" yaml:"oauth2_service_client_secret"`

	AuthorizationServiceReadURI  string `env:"AUTHORIZATION_SERVICE_READ_URI" yaml:"authorization_service_read_uri"`
	AuthorizationServiceWriteURI string `env:"AUTHORIZATION_SERVICE_WRITE_URI" yaml:"authorization_service_write_uri"`

	DatabasePrimaryURL             []string `env:"DATABASE_URL" yaml:"database_url"`
	DatabaseReplicaURL             []string `env:"REPLICA_DATABASE_URL" yaml:"replica_database_url"`
	DatabaseMigrate                bool     `envDefault:"false" env:"DO_MIGRATION" yaml:"do_migration"`
	DatabaseMigrationPath          string   `envDefault:"./migrations/0001" env:"MIGRATION_PATH" yaml:"migration_path"`
	DatabaseSkipDefaultTransaction bool     `envDefault:"true" env:"SKIP_DEFAULT_TRANSACTION" yaml:"skip_default_transaction"`
	DatabasePreferSimpleProtocol   bool     `envDefault:"true" env:"PREFER_SIMPLE_PROTOCOL" yaml:"prefer_simple_protocol"`

	DatabaseMaxIdleConnections           int `envDefault:"2" env:"DATABASE_MAX_IDLE_CONNECTIONS" yaml:"database_max_idle_connections"`
	DatabaseMaxOpenConnections           int `envDefault:"5" env:"DATABASE_MAX_OPEN_CONNECTIONS" yaml:"database_max_open_connections"`
	DatabaseMaxConnectionLifeTimeSeconds int `envDefault:"300" env:"DATABASE_MAX_CONNECTION_LIFE_TIME_IN_SECONDS" yaml:"database_max_connection_life_time_seconds"`

	DatabaseTraceQueries          bool   `envDefault:"false" env:"DATABASE_LOG_QUERIES" yaml:"database_log_queries"`
	DatabaseSlowQueryLogThreshold string `envDefault:"200ms" env:"DATABASE_SLOW_QUERY_THRESHOLD" yaml:"database_slow_query_threshold"`

	EventsQueueName string `envDefault:"frame.events.internal_._queue" env:"EVENTS_QUEUE_NAME" yaml:"events_queue_name"`
	EventsQueueUrl  string `envDefault:"mem://frame.events.internal_._queue" env:"EVENTS_QUEUE_URL" yaml:"events_queue_url"`

	oidcMap OIDCMap `env:"-" yaml:"-"`
}

type ConfigurationSecurity interface {
	IsRunSecurely() bool
}

var _ ConfigurationSecurity = new(ConfigurationDefault)

func (c *ConfigurationDefault) IsRunSecurely() bool {
	return c.RunServiceSecurely
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

type ConfigurationPorts interface {
	Port() string
	HttpPort() string
	GrpcPort() string
}

var _ ConfigurationPorts = new(ConfigurationDefault)

func (c *ConfigurationDefault) Port() string {
	if i, err := strconv.Atoi(c.ServerPort); err == nil && i > 0 {
		return fmt.Sprintf(":%s", strings.TrimSpace(c.ServerPort))
	}

	if strings.HasPrefix(":", c.ServerPort) || strings.Contains(c.ServerPort, ":") {
		return c.ServerPort
	}

	return ":80"
}

func (c *ConfigurationDefault) HttpPort() string {

	if i, err := strconv.Atoi(c.HttpServerPort); err == nil && i > 0 {
		return fmt.Sprintf(":%s", strings.TrimSpace(c.HttpServerPort))
	}

	if strings.HasPrefix(":", c.HttpServerPort) || strings.Contains(c.HttpServerPort, ":") {
		return c.HttpServerPort
	}

	return c.Port()
}

func (c *ConfigurationDefault) GrpcPort() string {

	if i, err := strconv.Atoi(c.GrpcServerPort); err == nil && i > 0 {
		return fmt.Sprintf(":%s", strings.TrimSpace(c.GrpcServerPort))
	}

	if strings.HasPrefix(":", c.GrpcServerPort) || strings.Contains(c.GrpcServerPort, ":") {
		return c.GrpcServerPort
	}

	return c.Port()
}

type ConfigurationCORS interface {
	IsCORSEnabled() bool
	IsCORSAllowCredentials() bool
	GetCORSAllowedHeaders() []string
	GetCORSExposedHeaders() []string
	GetCORSAllowedOrigins() []string
	GetCORSAllowedMethods() []string
	GetCORSMaxAge() int
}

var _ ConfigurationCORS = new(ConfigurationDefault)

func (c *ConfigurationDefault) IsCORSEnabled() bool {
	return c.CORSEnabled
}

func (c *ConfigurationDefault) IsCORSAllowCredentials() bool {
	return c.CORSAllowCredentials
}
func (c *ConfigurationDefault) GetCORSMaxAge() int {
	return c.CORSMaxAge
}

func (c *ConfigurationDefault) GetCORSAllowedHeaders() []string {
	return c.CORSAllowedHeaders
}
func (c *ConfigurationDefault) GetCORSExposedHeaders() []string {
	return c.CORSExposedHeaders
}
func (c *ConfigurationDefault) GetCORSAllowedOrigins() []string {
	return c.CORSAllowedOrigins
}
func (c *ConfigurationDefault) GetCORSAllowedMethods() []string {
	return c.CORSAllowedMethods
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
	GetOauth2ServiceClientSecret() string
	GetOauth2ServiceAudience() string
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
	return val.(string)
}
func (c *ConfigurationDefault) GetOauth2WellKnownJwkData() string {
	return c.Oauth2WellKnownJwkData
}
func (c *ConfigurationDefault) GetOauth2Issuer() string {
	val, ok := c.oidcMap["issuer"]
	if !ok {
		return ""
	}
	return val.(string)
}
func (c *ConfigurationDefault) GetOauth2AuthorizationEndpoint() string {
	val, ok := c.oidcMap["authorization_endpoint"]
	if !ok {
		return ""
	}
	return val.(string)
}
func (c *ConfigurationDefault) GetOauth2RegistrationEndpoint() string {
	val, ok := c.oidcMap["registration_endpoint"]
	if !ok {
		return ""
	}
	return val.(string)
}
func (c *ConfigurationDefault) GetOauth2TokenEndpoint() string {
	val, ok := c.oidcMap["token_endpoint"]
	if !ok {
		return ""
	}
	return val.(string)
}
func (c *ConfigurationDefault) GetOauth2UserInfoEndpoint() string {
	val, ok := c.oidcMap["userinfo_endpoint"]
	if !ok {
		return ""
	}
	return val.(string)
}
func (c *ConfigurationDefault) GetOauth2RevocationEndpoint() string {
	val, ok := c.oidcMap["revocation_endpoint"]
	if !ok {
		return ""
	}
	return val.(string)
}
func (c *ConfigurationDefault) GetOauth2EndSessionEndpoint() string {
	val, ok := c.oidcMap["end_session_endpoint"]
	if !ok {
		return ""
	}
	return val.(string)
}

func (c *ConfigurationDefault) GetOauth2ServiceURI() string {
	return c.Oauth2ServiceURI
}

func (c *ConfigurationDefault) GetOauth2ServiceClientSecret() string {
	return c.Oauth2ServiceClientSecret
}
func (c *ConfigurationDefault) GetOauth2ServiceAudience() string {
	return c.Oauth2ServiceAudience
}
func (c *ConfigurationDefault) GetOauth2ServiceAdminURI() string {
	return c.Oauth2ServiceAdminURI
}

type ConfigurationAuthorization interface {
	GetAuthorizationServiceReadURI() string
	GetAuthorizationServiceWriteURI() string
}

var _ ConfigurationAuthorization = new(ConfigurationDefault)

func (c *ConfigurationDefault) GetAuthorizationServiceReadURI() string {
	return c.AuthorizationServiceReadURI
}

func (c *ConfigurationDefault) GetAuthorizationServiceWriteURI() string {
	return c.AuthorizationServiceWriteURI
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
	CanDatabaseTraceQueries() bool
	GetDatabaseSlowQueryLogThreshold() time.Duration
}

var _ ConfigurationDatabase = new(ConfigurationDefault)

func (c *ConfigurationDefault) GetDatabasePrimaryHostURL() []string {
	return c.DatabasePrimaryURL
}

func (c *ConfigurationDefault) GetDatabaseReplicaHostURL() []string {
	return c.DatabasePrimaryURL
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
func (c *ConfigurationDefault) CanDatabaseTraceQueries() bool {
	return c.DatabaseTraceQueries
}
func (c *ConfigurationDefault) GetDatabaseSlowQueryLogThreshold() time.Duration {
	threshold, err := time.ParseDuration(c.DatabaseSlowQueryLogThreshold)
	if err != nil {
		threshold = 200 * time.Millisecond
	}
	return threshold
}

type ConfigurationEvents interface {
	GetEventsQueueName() string
	GetEventsQueueUrl() string
}

var _ ConfigurationEvents = new(ConfigurationDefault)

func (c *ConfigurationDefault) GetEventsQueueName() string {
	return c.EventsQueueName
}

func (c *ConfigurationDefault) GetEventsQueueUrl() string {
	return c.EventsQueueUrl
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

	hresp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(hresp.Body)

	if hresp.StatusCode/100 != 2 {
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

	hresp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(hresp.Body)

	if hresp.StatusCode/100 != 2 {
		return "", fmt.Errorf("JWKs data request %q failed: %d %s", url, hresp.StatusCode, hresp.Status)
	}

	var jwkData []byte
	jwkData, err = io.ReadAll(hresp.Body)
	jwkString := string(jwkData)

	return jwkString, err
}
