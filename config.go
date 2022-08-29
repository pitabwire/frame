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

type Configuration struct {
	Oauth2WellKnownJwk           string `envconfig:"OAUTH2_WELL_KNOWN_JWK"`
	AuthorizationServiceReadURI  string `envconfig:"AUTHORIZATION_SERVICE_READ_URI"`
	AuthorizationServiceWriteURI string `envconfig:"AUTHORIZATION_SERVICE_WRITE_URI"`
}
