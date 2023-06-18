package frame

import (
	"os"
	"strconv"
	"strings"
)

// Config Option that helps to specify or override the configuration object of our service.
func Config(config interface{}) Option {
	return func(s *Service) {
		s.configuration = config
	}
}

func (s *Service) Config() interface{} {
	return s.configuration
}

type ConfigurationDefault struct {
	ServerPort     string `default:"7000" envconfig:"PORT"`
	GrpcServerPort string `default:":50051" envconfig:"GRPC_PORT"`

	TLSCertificatePath             string `envconfig:"TLS_CERTIFICATE_PATH"`
	TLSCertificateKeyPath          string `envconfig:"TLS_CERTIFICATE_KEY_PATH"`
	TLSCertificateDomains          string `envconfig:"TLS_CERTIFICATE_DOMAINS"`
	TLSCertificateCommonName       string `envconfig:"TLS_CERTIFICATE_COMMON_NAME"`
	TLSCertificateCountry          string `envconfig:"TLS_CERTIFICATE_COUNTRY"`
	TLSCertificateOrganization     string `envconfig:"TLS_CERTIFICATE_ORGANIZATION"`
	TLSCertificateOrganizationUnit string `default:"computing" envconfig:"TLS_CERTIFICATE_ORGANIZATION_UNIT"`
	TLSCertificateValidYears       string `default:"0" envconfig:"TLS_CERTIFICATE_VALID_YEARS"`
	TLSCertificateValidMonths      string `default:"1" envconfig:"TLS_CERTIFICATE_VALID_MONTHS"`
	TLSCertificateValidDays        string `default:"0" envconfig:"TLS_CERTIFICATE_VALID_DAYS"`

	Oauth2WellKnownJwk        string `envconfig:"OAUTH2_WELL_KNOWN_JWK"`
	Oauth2JwtVerifyAudience   string `envconfig:"OAUTH2_JWT_VERIFY_AUDIENCE"`
	Oauth2JwtVerifyIssuer     string `envconfig:"OAUTH2_JWT_VERIFY_ISSUER"`
	Oauth2ServiceURI          string `envconfig:"OAUTH2_SERVICE_URI"`
	Oauth2ServiceClientSecret string `envconfig:"OAUTH2_SERVICE_CLIENT_SECRET"`
	Oauth2ServiceAudience     string `envconfig:"OAUTH2_SERVICE_AUDIENCE"`
	Oauth2ServiceAdminURI     string `envconfig:"OAUTH2_SERVICE_ADMIN_URI"`

	AuthorizationServiceReadURI  string `envconfig:"AUTHORIZATION_SERVICE_READ_URI"`
	AuthorizationServiceWriteURI string `envconfig:"AUTHORIZATION_SERVICE_WRITE_URI"`

	DatabasePrimaryURL    string `envconfig:"DATABASE_URL"`
	DatabaseReplicaURL    string `envconfig:"REPLICA_DATABASE_URL"`
	DatabaseMigrate       string `default:"false" envconfig:"DO_MIGRATION"`
	DatabaseMigrationPath string `default:"./migrations/0001" envconfig:"MIGRATION_PATH"`

	EventsQueueName string `default:"frame.events.internal_._queue" envconfig:"EVENTS_QUEUE_NAME"`
	EventsQueueUrl  string `default:"mem://frame.events.internal_._queue" envconfig:"EVENTS_QUEUE_URL"`
}

type ConfigurationPort interface {
	Port() string
}

func (c *ConfigurationDefault) Port() string {
	return c.ServerPort
}

type ConfigurationGrpcPort interface {
	GrpcPort() string
}

func (c *ConfigurationDefault) GrpcPort() string {
	return c.GrpcServerPort
}

type ConfigurationOAUTH2 interface {
	GetOauthWellKnownJwk() string
	GetOauth2ServiceURI() string
	GetOauth2ServiceClientSecret() string
	GetOauth2ServiceAudience() string
	GetOauth2ServiceAdminURI() string
}

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

func (c *ConfigurationDefault) GetAuthorizationServiceReadURI() string {
	return c.AuthorizationServiceReadURI
}

func (c *ConfigurationDefault) GetAuthorizationServiceWriteURI() string {
	return c.AuthorizationServiceWriteURI
}

type ConfigurationDatabase interface {
	GetDatabasePrimaryHostURL() string
	GetDatabaseReplicaHostURL() string
	DoDatabaseMigrate() bool
	GetDatabaseMigrationPath() string
}

func (c *ConfigurationDefault) GetDatabasePrimaryHostURL() string {
	return c.DatabasePrimaryURL
}

func (c *ConfigurationDefault) GetDatabaseReplicaHostURL() string {
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

func (c *ConfigurationDefault) GetDatabaseMigrationPath() string {
	return c.DatabaseMigrationPath
}

type ConfigurationEvents interface {
	GetEventsQueueName() string
	GetEventsQueueUrl() string
}

func (c *ConfigurationDefault) GetEventsQueueName() string {
	return c.EventsQueueName
}

func (c *ConfigurationDefault) GetEventsQueueUrl() string {
	return c.EventsQueueUrl
}

type ConfigurationTLS interface {
	TLSCertDomains() []string
	TLSCertPath() string
	TLSCertKeyPath() string
	TLSCertCommonName() string
	TLSCertCountry() string
	TLSCertOrganization() string
	TLSCertOrganizationUnit() string
	TLSCertValidYears() int
	TLSCertValidMonths() int
	TLSCertValidDays() int
	SetTLSCertAndKeyPath(certificatePath, certificateKeyPath string)
}

func (c *ConfigurationDefault) TLSCertDomains() []string {
	return strings.Split(c.TLSCertificateDomains, ",")
}

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

func (c *ConfigurationDefault) TLSCertCommonName() string {
	return c.TLSCertificateCommonName
}
func (c *ConfigurationDefault) TLSCertCountry() string {
	return c.TLSCertificateCountry
}
func (c *ConfigurationDefault) TLSCertOrganization() string {
	return c.TLSCertificateOrganization
}
func (c *ConfigurationDefault) TLSCertOrganizationUnit() string {
	return c.TLSCertificateOrganizationUnit
}
func (c *ConfigurationDefault) TLSCertValidYears() int {
	duration, err := strconv.Atoi(c.TLSCertificateValidYears)
	if err != nil {
		return 0
	}
	return duration
}
func (c *ConfigurationDefault) TLSCertValidMonths() int {
	duration, err := strconv.Atoi(c.TLSCertificateValidMonths)
	if err != nil {
		return 0
	}
	return duration
}
func (c *ConfigurationDefault) TLSCertValidDays() int {
	duration, err := strconv.Atoi(c.TLSCertificateValidDays)
	if err != nil {
		return 0
	}
	return duration
}
