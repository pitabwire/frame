package frame

import "github.com/kelseyhightower/envconfig"

// Config Option that helps to specify or override the configuration object of our service
func Config(config interface{}) Option {
	return func(s *Service) {
		s.configuration = config
	}
}

func (s *Service) Config() interface{} {
	return s.configuration
}

func (s *Service) ProcessConfig(prefix string, config interface{}) error {
	return envconfig.Process(prefix, config)
}

type DefaultConfiguration struct {
	ServerPort string `envconfig:"PORT"`
}

type OAUTH2Configuration struct {
	Oauth2WellKnownJwk           string `envconfig:"OAUTH2_WELL_KNOWN_JWK"`
	AuthorizationServiceReadURI  string `envconfig:"AUTHORIZATION_SERVICE_READ_URI"`
	AuthorizationServiceWriteURI string `envconfig:"AUTHORIZATION_SERVICE_WRITE_URI"`
	Oauth2JwtVerifyAudience      string `envconfig:"OAUTH2_JWT_VERIFY_AUDIENCE"`
	Oauth2JwtVerifyIssuer        string `envconfig:"OAUTH2_JWT_VERIFY_ISSUER"`
	Oauth2ServiceURI             string `envconfig:"OAUTH2_SERVICE_URI"`
	Oauth2ServiceClientSecret    string `envconfig:"OAUTH2_SERVICE_CLIENT_SECRET"`
	Oauth2ServiceAudience        string `envconfig:"OAUTH2_SERVICE_AUDIENCE"`
}

type DatabaseConfiguration struct {
	EnvDatabaseURL        string `envconfig:"DATABASE_URL"`
	EnvReplicaDatabaseURL string `envconfig:"REPLICA_DATABASE_URL"`
	EnvMigrate            string `envconfig:"DO_MIGRATION"`
	EnvMigrationPath      string `envconfig:"MIGRATION_PATH"`
}
