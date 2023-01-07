package frame

import (
	"os"
	"strconv"
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
	ServerPort string `default:"7000" envconfig:"PORT"`

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

func (c *ConfigurationDefault) GetAuthorizationServiceReadURI() string {
	return c.AuthorizationServiceReadURI
}

func (c *ConfigurationDefault) GetAuthorizationServiceWriteURI() string {
	return c.AuthorizationServiceWriteURI
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

type ConfigurationOAUTH2 interface {
	GetOauthWellKnownJwk() string
	GetOauth2ServiceURI() string
	GetOauth2ServiceClientSecret() string
	GetOauth2ServiceAudience() string
	GetOauth2ServiceAdminURI() string
}

type ConfigurationAuthorization interface {
	GetAuthorizationServiceReadURI() string
	GetAuthorizationServiceWriteURI() string
}

type ConfigurationDatabase interface {
	GetDatabasePrimaryHostURL() string
	GetDatabaseReplicaHostURL() string
	DoDatabaseMigrate() bool
	GetDatabaseMigrationPath() string
}
