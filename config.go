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

func (c *ConfigurationDefault) GetAuthorizationServiceReadURI() string {
	return c.AuthorizationServiceReadURI
}

func (c *ConfigurationDefault) GetAuthorizationServiceWriteURI() string {
	return c.AuthorizationServiceWriteURI
}

func (c *ConfigurationDefault) GetDatabasePrimaryHostUrl() string {
	return c.DatabasePrimaryURL
}

func (c *ConfigurationDefault) GetDatabaseReplicaHostUrl() string {
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
}

type ConfigurationAuthorization interface {
	GetAuthorizationServiceReadURI() string
	GetAuthorizationServiceWriteURI() string
}

type ConfigurationDatabase interface {
	GetDatabasePrimaryHostUrl() string
	GetDatabaseReplicaHostUrl() string
	DoDatabaseMigrate() bool
	GetDatabaseMigrationPath() string
}
