package frame

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config Option that helps to specify or override the configuration object of our service.
func Config(config any) Option {
	return func(s *Service) {
		s.configuration = config
	}
}

func (s *Service) Config() any {
	return s.configuration
}

type ConfigurationDefault struct {
	LogLevel string `default:"info" envconfig:"LOG_LEVEL"`

	ServerPort     string `default:":7000" envconfig:"PORT"`
	HttpServerPort string `default:":8080" envconfig:"HTTP_PORT"`
	GrpcServerPort string `default:":50051" envconfig:"GRPC_PORT"`

	CORSEnabled          bool     `default:"false" envconfig:"CORS_ENABLED"`
	CORSAllowCredentials bool     `default:"false" envconfig:"CORS_ALLOW_CREDENTIALS"`
	CORSAllowedHeaders   []string `default:"Authorization" envconfig:"CORS_ALLOWED_HEADERS"`
	CORSExposedHeaders   []string `default:"*" envconfig:"CORS_EXPOSED_HEADERS"`
	CORSAllowedOrigins   []string `default:"*" envconfig:"CORS_ALLOWED_ORIGINS"`
	CORSAllowedMethods   []string `default:"GET,HEAD,POST,PUT,OPTIONS" envconfig:"CORS_ALLOWED_METHODS"`
	CORSMaxAge           int      `default:"3600" envconfig:"CORS_MAX_AGE"`

	TLSCertificatePath    string `envconfig:"TLS_CERTIFICATE_PATH"`
	TLSCertificateKeyPath string `envconfig:"TLS_CERTIFICATE_KEY_PATH"`

	Oauth2WellKnownJwk        string `envconfig:"OAUTH2_WELL_KNOWN_JWK"`
	Oauth2JwtVerifyAudience   string `envconfig:"OAUTH2_JWT_VERIFY_AUDIENCE"`
	Oauth2JwtVerifyIssuer     string `envconfig:"OAUTH2_JWT_VERIFY_ISSUER"`
	Oauth2ServiceURI          string `envconfig:"OAUTH2_SERVICE_URI"`
	Oauth2ServiceClientSecret string `envconfig:"OAUTH2_SERVICE_CLIENT_SECRET"`
	Oauth2ServiceAudience     string `envconfig:"OAUTH2_SERVICE_AUDIENCE"`
	Oauth2ServiceAdminURI     string `envconfig:"OAUTH2_SERVICE_ADMIN_URI"`

	AuthorizationServiceReadURI  string `envconfig:"AUTHORIZATION_SERVICE_READ_URI"`
	AuthorizationServiceWriteURI string `envconfig:"AUTHORIZATION_SERVICE_WRITE_URI"`

	DatabasePrimaryURL             []string `envconfig:"DATABASE_URL"`
	DatabaseReplicaURL             []string `envconfig:"REPLICA_DATABASE_URL"`
	DatabaseMigrate                string   `default:"false" envconfig:"DO_MIGRATION"`
	DatabaseMigrationPath          string   `default:"./migrations/0001" envconfig:"MIGRATION_PATH"`
	DatabaseSkipDefaultTransaction bool     `default:"true" envconfig:"SKIP_DEFAULT_TRANSACTION"`

	DatabaseMaxIdleConnections           int `default:"20" envconfig:"DATABASE_MAX_IDLE_CONNECTIONS"`
	DatabaseMaxOpenConnections           int `default:"200" envconfig:"DATABASE_MAX_OPEN_CONNECTIONS"`
	DatabaseMaxConnectionLifeTimeSeconds int `default:"300" envconfig:"DATABASE_MAX_CONNECTION_LIFE_TIME_IN_SECONDS"`

	EventsQueueName string `default:"frame.events.internal_._queue" envconfig:"EVENTS_QUEUE_NAME"`
	EventsQueueUrl  string `default:"mem://frame.events.internal_._queue" envconfig:"EVENTS_QUEUE_URL"`
}

type ConfigurationLogLevel interface {
	LoggingLevel() string
	LoggingLevelIsDebug() bool
}

var _ ConfigurationLogLevel = new(ConfigurationDefault)

func (c *ConfigurationDefault) LoggingLevel() string {
	return strings.ToLower(c.LogLevel)
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
	GetOauthWellKnownJwk() string
	GetOauth2ServiceURI() string
	GetOauth2ServiceClientSecret() string
	GetOauth2ServiceAudience() string
	GetOauth2ServiceAdminURI() string
}

var _ ConfigurationOAUTH2 = new(ConfigurationDefault)

func (c *ConfigurationDefault) GetOauthWellKnownJwk() string {
	return c.Oauth2WellKnownJwk
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
	GetMaxIdleConnections() int
	GetMaxOpenConnections() int
	GetMaxConnectionLifeTimeInSeconds() time.Duration

	GetDatabaseMigrationPath() string
}

var _ ConfigurationDatabase = new(ConfigurationDefault)

func (c *ConfigurationDefault) GetDatabasePrimaryHostURL() []string {
	return c.DatabasePrimaryURL
}

func (c *ConfigurationDefault) GetDatabaseReplicaHostURL() []string {
	return c.DatabasePrimaryURL
}

func (c *ConfigurationDefault) DoDatabaseMigrate() bool {
	isMigration, err := strconv.ParseBool(c.DatabaseMigrate)
	if err != nil {
		isMigration = false
	}

	stdArgs := os.Args[1:]
	return isMigration || (len(stdArgs) > 0 && stdArgs[0] == "migrate")
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
