package config //nolint:testpackage // tests access unexported ctxKeyConfiguration and OIDCMap

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
}

func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}

func (s *ConfigSuite) TestContextHelpersAndKeyString() {
	ctx := context.Background()
	cfg := ConfigurationDefault{ServiceName: "svc"}

	s.Equal("frame/config/configurationKey", ctxKeyConfiguration.String())

	ctx = ToContext(ctx, cfg)
	fromCtx := FromContext[ConfigurationDefault](ctx)
	s.Equal("svc", fromCtx.ServiceName)

	missing := FromContext[*ConfigurationDefault](context.Background())
	s.Nil(missing)
}

func (s *ConfigSuite) TestFromEnvAndFillEnv() {
	type envCfg struct {
		Value string `env:"FRAME_TEST_VALUE"`
	}

	s.T().Setenv("FRAME_TEST_VALUE", "abc")

	fromEnv, err := FromEnv[envCfg]()
	s.Require().NoError(err)
	s.Equal("abc", fromEnv.Value)

	var target envCfg
	s.Require().NoError(FillEnv(&target))
	s.Equal("abc", target.Value)
}

func (s *ConfigSuite) TestCoreGettersAndBooleans() {
	cfg := &ConfigurationDefault{
		ServiceName:                       "svc",
		ServiceEnvironment:                "prod",
		ServiceVersion:                    "1.2.3",
		RunServiceSecurely:                true,
		LogLevel:                          "trace",
		LogFormat:                         "json",
		LogTimeFormat:                     time.RFC3339,
		LogColored:                        true,
		LogShowStackTrace:                 true,
		TraceRequests:                     true,
		TraceRequestsLogBody:              true,
		ProfilerEnable:                    true,
		ProfilerPortAddr:                  ":7001",
		OpenTelemetryDisable:              true,
		OpenTelemetryTraceRatio:           0.42,
		WorkerPoolCPUFactorForWorkerCount: 3,
		WorkerPoolCapacity:                64,
		WorkerPoolCount:                   8,
		WorkerPoolExpiryDuration:          "2s",
	}

	s.Equal("svc", cfg.Name())
	s.Equal("prod", cfg.Environment())
	s.Equal("1.2.3", cfg.Version())
	s.True(cfg.IsRunSecurely())
	s.Equal("trace", cfg.LoggingLevel())
	s.Equal("json", cfg.LoggingFormat())
	s.Equal(time.RFC3339, cfg.LoggingTimeFormat())
	s.True(cfg.LoggingColored())
	s.True(cfg.LoggingShowStackTrace())
	s.True(cfg.LoggingLevelIsDebug())
	s.True(cfg.TraceReq())
	s.True(cfg.TraceReqLogBody())
	s.True(cfg.ProfilerEnabled())
	s.Equal(":7001", cfg.ProfilerPort())
	s.True(cfg.DisableOpenTelemetry())
	s.InEpsilon(0.42, cfg.SamplingRatio(), 1e-9)
	s.Equal(3, cfg.GetCPUFactor())
	s.Equal(64, cfg.GetCapacity())
	s.Equal(8, cfg.GetCount())
	s.Equal(2*time.Second, cfg.GetExpiryDuration())
}

func (s *ConfigSuite) TestFallbacksTable() {
	testCases := []struct {
		name        string
		cfg         ConfigurationDefault
		wantPort    string
		wantHTTP    string
		wantProfile string
		wantExpiry  time.Duration
	}{
		{
			name: "numeric ports",
			cfg: ConfigurationDefault{
				ServerPort:               "7000",
				HTTPServerPort:           "8080",
				ProfilerPortAddr:         ":6600",
				WorkerPoolExpiryDuration: "1500ms",
			},
			wantPort:    ":7000",
			wantHTTP:    ":8080",
			wantProfile: ":6600",
			wantExpiry:  1500 * time.Millisecond,
		},
		{
			name: "invalid ports fallback",
			cfg: ConfigurationDefault{
				ServerPort:               "invalid",
				HTTPServerPort:           "invalid",
				ProfilerPortAddr:         "",
				WorkerPoolExpiryDuration: "invalid",
			},
			wantPort:    ":80",
			wantHTTP:    ":8080",
			wantProfile: ":6060",
			wantExpiry:  time.Second,
		},
		{
			name: "already host-bound",
			cfg: ConfigurationDefault{
				ServerPort:               "0.0.0.0:7000",
				HTTPServerPort:           ":8088",
				WorkerPoolExpiryDuration: "1s",
			},
			wantPort:    "0.0.0.0:7000",
			wantHTTP:    ":8088",
			wantProfile: ":6060",
			wantExpiry:  time.Second,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.Equal(tc.wantPort, tc.cfg.Port())
			s.Equal(tc.wantHTTP, tc.cfg.HTTPPort())
			s.Equal(tc.wantProfile, tc.cfg.ProfilerPort())
			s.Equal(tc.wantExpiry, tc.cfg.GetExpiryDuration())
		})
	}
}

func (s *ConfigSuite) TestHTTPServerSettings() {
	cfg := &ConfigurationDefault{
		HTTPServerReadTimeout:       "31s",
		HTTPServerReadHeaderTimeout: "6s",
		HTTPServerWriteTimeout:      "32s",
		HTTPServerIdleTimeout:       "91s",
		HTTPServerShutdownTimeout:   "16s",
		HTTPServerMaxHeaderBytes:    2048,
	}

	s.Equal(31*time.Second, cfg.HTTPReadTimeout())
	s.Equal(6*time.Second, cfg.HTTPReadHeaderTimeout())
	s.Equal(32*time.Second, cfg.HTTPWriteTimeout())
	s.Equal(91*time.Second, cfg.HTTPIdleTimeout())
	s.Equal(16*time.Second, cfg.HTTPShutdownTimeout())
	s.Equal(2048*1024, cfg.HTTPMaxHeaderBytes())

	cfg.HTTPServerReadTimeout = "invalid"
	cfg.HTTPServerReadHeaderTimeout = "invalid"
	cfg.HTTPServerWriteTimeout = "invalid"
	cfg.HTTPServerIdleTimeout = "invalid"
	cfg.HTTPServerShutdownTimeout = "invalid"
	cfg.HTTPServerMaxHeaderBytes = 0

	s.Equal(30*time.Second, cfg.HTTPReadTimeout())
	s.Equal(5*time.Second, cfg.HTTPReadHeaderTimeout())
	s.Equal(30*time.Second, cfg.HTTPWriteTimeout())
	s.Equal(90*time.Second, cfg.HTTPIdleTimeout())
	s.Equal(15*time.Second, cfg.HTTPShutdownTimeout())
	s.Equal(1024*1024, cfg.HTTPMaxHeaderBytes())
}

func (s *ConfigSuite) TestDatabaseAndEventConfig() {
	cfg := &ConfigurationDefault{
		DatabasePrimaryURL:                   []string{"postgres://primary"},
		DatabaseReplicaURL:                   []string{"postgres://replica"},
		DatabaseMigrate:                      false,
		DatabaseSkipDefaultTransaction:       true,
		DatabasePreferSimpleProtocol:         true,
		DatabaseMaxIdleConnections:           4,
		DatabaseMaxOpenConnections:           9,
		DatabaseMaxConnectionLifeTimeSeconds: 321,
		DatabaseMigrationPath:                "./migrations",
		DatabaseTraceQueries:                 true,
		DatabaseSlowQueryLogThreshold:        "450ms",
		EventsQueueName:                      "",
		EventsQueueURL:                       "",
	}

	origArgs := os.Args
	s.T().Cleanup(func() { os.Args = origArgs }) //nolint:reassign // intentional: restore os.Args after test

	os.Args = []string{"bin", "start"} //nolint:reassign // intentional: test CLI arg parsing
	s.False(cfg.DoDatabaseMigrate())
	os.Args = []string{"bin", "migrate"} //nolint:reassign // intentional: test CLI arg parsing
	s.True(cfg.DoDatabaseMigrate())

	s.Equal([]string{"postgres://primary"}, cfg.GetDatabasePrimaryHostURL())
	s.Equal([]string{"postgres://replica"}, cfg.GetDatabaseReplicaHostURL())
	s.True(cfg.SkipDefaultTransaction())
	s.True(cfg.PreferSimpleProtocol())
	s.Equal(4, cfg.GetMaxIdleConnections())
	s.Equal(9, cfg.GetMaxOpenConnections())
	s.Equal(321*time.Second, cfg.GetMaxConnectionLifeTimeInSeconds())
	s.Equal("./migrations", cfg.GetDatabaseMigrationPath())
	s.True(cfg.CanDatabaseTraceQueries())
	s.Equal(450*time.Millisecond, cfg.GetDatabaseSlowQueryLogThreshold())
	s.Equal("frame.events.internal_._queue", cfg.GetEventsQueueName())
	s.Equal("mem://frame.events.internal_._queue", cfg.GetEventsQueueURL())

	cfg.DatabaseSlowQueryLogThreshold = "invalid"
	s.Equal(DefaultSlowQueryThreshold, cfg.GetDatabaseSlowQueryLogThreshold())
	cfg.EventsQueueName = "events"
	cfg.EventsQueueURL = "nats://localhost:4222"
	s.Equal("events", cfg.GetEventsQueueName())
	s.Equal("nats://localhost:4222", cfg.GetEventsQueueURL())
}

func (s *ConfigSuite) TestTLSAndAuthorizationGetters() {
	cfg := &ConfigurationDefault{
		AuthorizationServiceReadURI:  "http://read",
		AuthorizationServiceWriteURI: "http://write",
	}

	cfg.SetTLSCertAndKeyPath("cert.pem", "key.pem")
	s.Equal("cert.pem", cfg.TLSCertPath())
	s.Equal("key.pem", cfg.TLSCertKeyPath())
	s.Equal("http://read", cfg.GetAuthorizationServiceReadURI())
	s.Equal("http://write", cfg.GetAuthorizationServiceWriteURI())
	s.True(cfg.AuthorizationServiceCanRead())
	s.True(cfg.AuthorizationServiceCanWrite())
}

func (s *ConfigSuite) TestOIDCLoadAndGetters() {
	oidcSrv := newTestOIDCServer(s.T(), false, false)
	cfg := &ConfigurationDefault{
		Oauth2ServiceURI:          oidcSrv.discoveryURLRoot(),
		Oauth2WellKnownOIDCPath:   ".well-known/openid-configuration",
		Oauth2ServiceClientID:     "client-id",
		Oauth2ServiceClientSecret: "client-secret",
		Oauth2ServiceAudience:     []string{"aud1"},
		Oauth2ServiceAdminURI:     "http://admin.local",
		Oauth2JwtVerifyAudience:   []string{"verifier"},
		Oauth2JwtVerifyIssuer:     "issuer",
	}

	s.Require().NoError(cfg.LoadOauth2Config(context.Background()))

	s.Equal(oidcSrv.jwksURL(), cfg.GetOauth2WellKnownJwk())
	s.NotEmpty(cfg.GetOauth2WellKnownJwkData())
	s.Equal("http://issuer.local", cfg.GetOauth2Issuer())
	s.Equal("http://auth.local", cfg.GetOauth2AuthorizationEndpoint())
	s.Equal("http://reg.local", cfg.GetOauth2RegistrationEndpoint())
	s.Equal("http://token.local", cfg.GetOauth2TokenEndpoint())
	s.Equal("http://userinfo.local", cfg.GetOauth2UserInfoEndpoint())
	s.Equal("http://revoke.local", cfg.GetOauth2RevocationEndpoint())
	s.Equal("http://logout.local", cfg.GetOauth2EndSessionEndpoint())
	s.Equal("client-id", cfg.GetOauth2ServiceClientID())
	s.Equal("client-secret", cfg.GetOauth2ServiceClientSecret())
	s.Empty(cfg.GetOauth2TokenEndpointAuthMethod())
	s.Nil(cfg.GetOauth2PrivateKeyJWTConfig())
	s.Equal([]string{"aud1"}, cfg.GetOauth2ServiceAudience())
	s.Equal("http://admin.local", cfg.GetOauth2ServiceAdminURI())
	s.Equal([]string{"verifier"}, cfg.GetVerificationAudience())
	s.Equal("issuer", cfg.GetVerificationIssuer())
}

func (s *ConfigSuite) TestPrivateKeyJWTGetters() {
	cfg := &ConfigurationDefault{
		Oauth2ServiceClientID:         "svc-client",
		Oauth2TokenEndpointAuthMethod: "private_key_jwt",
		Oauth2PrivateJwtKey: OAuth2PrivateJWTKeyConfig{
			PrivateKeyPEM: "pem-data",
			KeyID:         "kid-1",
			Audience:      "https://issuer.local/oauth2/token",
			Issuer:        "issuer-id",
			Subject:       "subject-id",
		},
	}

	privateKeyJWT := cfg.GetOauth2PrivateKeyJWTConfig()
	s.Require().NotNil(privateKeyJWT)
	s.Equal("private_key_jwt", cfg.GetOauth2TokenEndpointAuthMethod())
	s.Equal([]byte("pem-data"), privateKeyJWT.PrivateKeyPEM)
	s.Equal("kid-1", privateKeyJWT.KeyID)
	s.Equal("https://issuer.local/oauth2/token", privateKeyJWT.Audience)
	s.Equal("issuer-id", privateKeyJWT.Issuer)
	s.Equal("subject-id", privateKeyJWT.Subject)
}

func (s *ConfigSuite) TestPrivateKeyJWTRequiresExplicitAuthMethod() {
	cfg := &ConfigurationDefault{
		Oauth2PrivateJwtKey: OAuth2PrivateJWTKeyConfig{
			Source:   PrivateKeyJWTSourceWorkloadAPI,
			SPIFFEID: "spiffe://example.org/ns/default/sa/service-authentication",
			Hint:     "internal",
		},
	}

	privateKeyJWT := cfg.GetOauth2PrivateKeyJWTConfig()
	s.Require().NotNil(privateKeyJWT)
	s.Empty(cfg.GetOauth2TokenEndpointAuthMethod(), "auth method should not be inferred")
	s.Equal(PrivateKeyJWTSourceWorkloadAPI, privateKeyJWT.Source)
	s.Equal("spiffe://example.org/ns/default/sa/service-authentication", privateKeyJWT.SPIFFEID)
	s.Equal("internal", privateKeyJWT.Hint)

	// With explicit auth method set, it is returned as-is.
	cfg.Oauth2TokenEndpointAuthMethod = TokenEndpointAuthMethodPrivateKeyJWT
	s.Equal(TokenEndpointAuthMethodPrivateKeyJWT, cfg.GetOauth2TokenEndpointAuthMethod())
}

func (s *ConfigSuite) TestPrivateKeyJWTEnvJSONParsing() {
	s.T().Setenv(
		"OAUTH2_PRIVATE_JWT_KEY",
		`{"source":"workload_api","spiffe_id":"spiffe://example.org/ns/default/sa/service-authentication","hint":"internal","key_id":"kid-1"}`,
	)

	type oidcCfg struct {
		ConfigurationDefault
	}

	cfg, err := FromEnv[oidcCfg]()
	s.Require().NoError(err)
	privateKeyJWT := cfg.GetOauth2PrivateKeyJWTConfig()
	s.Require().NotNil(privateKeyJWT)
	s.Equal(PrivateKeyJWTSourceWorkloadAPI, privateKeyJWT.Source)
	s.Equal("spiffe://example.org/ns/default/sa/service-authentication", privateKeyJWT.SPIFFEID)
	s.Equal("internal", privateKeyJWT.Hint)
	s.Equal("kid-1", privateKeyJWT.KeyID)
}

func (s *ConfigSuite) TestOIDCMapTypeGuardsAndLoadErrors() {
	cfg := &ConfigurationDefault{
		Oauth2ServiceURI:        "http://127.0.0.1",
		Oauth2WellKnownOIDCPath: ".well-known/openid-configuration",
		oidcMap: OIDCMap{
			"jwks_uri":               10,
			"issuer":                 11,
			"authorization_endpoint": 12,
			"registration_endpoint":  13,
			"token_endpoint":         14,
			"userinfo_endpoint":      15,
			"revocation_endpoint":    16,
			"end_session_endpoint":   17,
		},
	}

	s.Empty(cfg.GetOauth2WellKnownJwk())
	s.Empty(cfg.GetOauth2Issuer())
	s.Empty(cfg.GetOauth2AuthorizationEndpoint())
	s.Empty(cfg.GetOauth2RegistrationEndpoint())
	s.Empty(cfg.GetOauth2TokenEndpoint())
	s.Empty(cfg.GetOauth2UserInfoEndpoint())
	s.Empty(cfg.GetOauth2RevocationEndpoint())
	s.Empty(cfg.GetOauth2EndSessionEndpoint())

	oidcSrv := newTestOIDCServer(s.T(), true, false)
	cfg.Oauth2ServiceURI = oidcSrv.discoveryURLRoot()
	s.Require().Error(cfg.LoadOauth2Config(context.Background()))

	oidcSrv = newTestOIDCServer(s.T(), false, true)
	cfg.Oauth2ServiceURI = oidcSrv.discoveryURLRoot()
	s.Require().Error(cfg.LoadOauth2Config(context.Background()))
}

func (s *ConfigSuite) TestLoadWithOIDC() {
	type sample struct {
		Value string `env:"CONFIG_TEST_LOAD_VALUE"`
	}
	s.T().Setenv("CONFIG_TEST_LOAD_VALUE", "x")
	cfg, err := LoadWithOIDC[sample](context.Background())
	s.Require().NoError(err)
	s.Equal("x", cfg.Value)

	type oidcCfg struct {
		ConfigurationDefault
	}

	oidcSrv := newTestOIDCServer(s.T(), false, false)
	s.T().Setenv("OAUTH2_SERVICE_URI", oidcSrv.discoveryURLRoot())
	s.T().Setenv("OAUTH2_WELL_KNOWN_OIDC_PATH", ".well-known/openid-configuration")

	loaded, err := LoadWithOIDC[oidcCfg](context.Background())
	s.Require().NoError(err)
	s.NotEmpty(loaded.GetOauth2WellKnownJwkData())
}

func (s *ConfigSuite) TestOIDCMapDirectLoaders() {
	ctx := context.Background()
	oid := OIDCMap{}

	s.Require().Error(oid.loadOIDC(ctx, "://bad-url"))
	_, err := oid.loadJWKData(ctx, "://bad-url")
	s.Require().Error(err)
}
