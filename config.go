package frame

// Config Option that helps to specify or override the configuration object of our service
func Config(config interface{}) Option {
	return func(s *Service) {
		s.configuration = config
	}
}

func (s *Service) Config() interface{} {
	return s.configuration
}

type DefaultConfiguration struct {
	ServerPort string `envconfig:"PORT"`

	Oauth2WellKnownJwk        string `envconfig:"OAUTH2_WELL_KNOWN_JWK"`
	Oauth2JwtVerifyAudience   string `envconfig:"OAUTH2_JWT_VERIFY_AUDIENCE"`
	Oauth2JwtVerifyIssuer     string `envconfig:"OAUTH2_JWT_VERIFY_ISSUER"`
	Oauth2ServiceURI          string `envconfig:"OAUTH2_SERVICE_URI"`
	Oauth2ServiceClientSecret string `envconfig:"OAUTH2_SERVICE_CLIENT_SECRET"`
	Oauth2ServiceAudience     string `envconfig:"OAUTH2_SERVICE_AUDIENCE"`

	AuthorizationServiceReadURI  string `envconfig:"AUTHORIZATION_SERVICE_READ_URI"`
	AuthorizationServiceWriteURI string `envconfig:"AUTHORIZATION_SERVICE_WRITE_URI"`
}

func (d DefaultConfiguration) GetOauthWellKnownJwk() string {
	return d.Oauth2WellKnownJwk
}

func (d DefaultConfiguration) GetOauth2ServiceURI() string {
	return d.Oauth2ServiceURI
}

func (d DefaultConfiguration) GetAuthorizationServiceReadURI() string {
	return d.AuthorizationServiceReadURI
}

func (d DefaultConfiguration) GetAuthorizationServiceWriteURI() string {
	return d.AuthorizationServiceWriteURI
}

type OAUTH2Configuration interface {
	GetOauthWellKnownJwk() string
	GetOauth2ServiceURI() string
}

type AuthorizationConfiguration interface {
	GetAuthorizationServiceReadURI() string
	GetAuthorizationServiceWriteURI() string
}

type DatabaseConfiguration struct {
	DatabaseURL        string `required:"true" envconfig:"DATABASE_URL"`
	ReplicaDatabaseURL string `envconfig:"REPLICA_DATABASE_URL"`
	Migrate            string `default:"false" envconfig:"DO_MIGRATION"`
	MigrationPath      string `default:"./migrations/0001" envconfig:"MIGRATION_PATH"`
}
