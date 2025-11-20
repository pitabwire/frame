package security_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testoryhydra"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/openid"
	"github.com/pitabwire/frame/tests"
)

const sampleAccessKey = "eyJhbGciOiJSUzI1NiIsImtpZCI6InB1YmxpYzpmODg2ZDBmNy0zYmY0LTQzMzgtOGU4Yy01ZjhjNmVlNGM3MWQiLCJ0eXAiOiJKV1QifQ.eyJhdF9oYXNoIjoicUdqdV91YnRuUkRyaGZ6WEppVzl3dyIsImF1ZCI6WyJjMmY0ajdhdTZzN2Y5MXVxbm9rZyJdLCJhdXRoX3RpbWUiOjE2MjIzMDk0OTUsImV4cCI6MTgwMjMwOTQ5OSwiaWF0IjoxNjIyMzA5NDk5LCJpc3MiOiJodHRwOi8vMTI3LjAuMC4xOjQ0NDQvIiwianRpIjoiZjM5ZGIzYTEtMmU3Ni00YzQyLWEyMmItMTg5NThiYTg3MjM1Iiwibm9uY2UiOiIiLCJyYXQiOjE2MjIzMDk0ODgsInNpZCI6ImNhNmM2NmE3LTg3MDItNDRjZS1hMTllLWRkZDJkYzQ4Y2E3MiIsInN1YiI6ImMyb2hoYzNuZGJtMGI2Y2g5dGUwIn0.BKh_m7fXaMlqXNLGisQ7vBtubgfws7h-oo9L_HXuUuY9mPs20dZ7HlQp_s-jxbdh1oDFxzRsoklbgmHglHCHBimDT3hkFPiZUmsqHtGM5P2neRBXD5ogWTjPBY_piIxu7JoB_GbFF1mZiy7Q7Lw_NpObvtLT3VC-wMMJ0fZDkyQY0hiFzLaUXVjJ96X0y0Vs0ExrcSQPnuT8CYQlhkO3qaRbKOM8p8C8IzHrmJg3N96IiZc8Vy9H9cbkmCfNlIvHx1zTIZbwyPbTjp43kI_Eo8fMmbdK_XkTnxouGtArVWoW1jjG6t4UgYafm42QJPJJvwIY2uwAg0x6B-1KwC9GgoxCGGWXRiWt9vL9ALxMpDRIxYqo2sh0OcVObvYsCTFKF8ekl5RSrvlAeu8QSkVXLvdBlaCHfvxHm2po32s6j7zvzXeuczxuiAj54Gd_7QWPwHu-2TW2gnG3oa5nbTofcmNb7Qm2QoGptIgx80gMJiCVGLCfv2UUwqZRoLzF9XkWiXKWRCq6dM4QYEIa6dyxT4BRb04W1Qcq_90Y8IsmWsXm3AQptILtDfEok93UIfnT5YnyDhAh4QmVlwCgzwokNlyd9vGtauKUZyIIKLyZ8GPCldou75GD7t4ZByUcRdHStuTvJEqJ98Fe85VolW8rubqIiN_uEzTNq5vWdFT5boo"

const sampleWellKnownJwk = "{\"keys\":[{\"use\":\"sig\",\"kty\":\"RSA\",\"kid\":\"public:f886d0f7-3bf4-4338-8e8c-5f8c6ee4c71d\",\"alg\":\"RS256\",\"n\":\"43g42b14fJGjB9wVMrYlk6L1Aig7HWt5Aere0AQQC3tdJdmzwvyCA4rYKDB2rTHSgN-xSWq12rtgrZIfjNHFj8w4p04U0aXhWFb_bVs0TTLrdlb9syAidX1H3JAwKngqHC3zkDRzVsKUKQCGSl2IScLR6B6eclgCsPL9O-SKA7BwdH3XHz3lFBhpc3Fn_TMMd2q44YQH8JWGKJPXiVHR6XB3w2IwNtFEbE3D2HpimRxP7GpGDMYFq1_eCFYUSdSK39dTfXj-SeQ0tM_voWNrS1ydH9eC5Au0zLaxWskco_8dKiGWYoQzep_Od4tlc_l2GDFoXhCOeb_6e3NENFGkef1ewPyX-hUguYm7OxBYwauBi6LTZYTkSHKocG1wT_XyE2QB7TEb2F1KQ_4WhLicGlOz4biVSs75v9FSPHFhMJ_ZpzjReYdTUJVBoQI2HMo5vElgxB79GgzCp-cp4286_OW1QnDfpvkgDIJtYnedNMmMWCpLTwtswYqO5lucCjR-jYukuV36NDuDOV1b-UqPQh95IScZUca6kVEU_5vcbiXaf24cDcVlMbN854HIYBzWinqrn_YX1mATrq6uoHT9Frth_pMJSop28iX5861p2dLdY0wBlb-x1YbQ8m5eoM7WMIDL1VVcoorecR6L_LS3App_XbaDSrtEE7wbS5iqyjk\",\"e\":\"AQAB\"},{\"use\":\"sig\",\"kty\":\"RSA\",\"kid\":\"public:0a7dad6b-ee8c-4d8a-8692-741770246f74\",\"alg\":\"RS256\",\"n\":\"x8zZj5GjuhJ4yABn2X1bCZi3jGJEIROqJxSNFt7lCi-IVMKbENudWL0HQxtnkglRitdAZXaiXToo5eWl-eWEIeK0p501PIX1Iq32BPehUI6H7t7Xth-0C65Ub2_Aho5QKCyXNH2mi75yyWIXLk0EWzgP_2H32BzS2w3OHjrogino7h0Neo98Q_727fKbTkreOLRyrvTNJWzpPrhoodz4UsT9EyY9eAW6kdaBl4k05qDm52BZM2PT4ToMeP3kMTFx_2aeoiegjaNkV2G5ONCLlYOHp3n8Hek_V_--695jaWvHgsWprykZ9KEX0nwhgy3B8DT329I9huf0vtjDqe1-Dd6qMwcMII8OT3i0_fp7rIlJUweufEIHpRVyR4KXfrsH8BP3V4Qyh_4o6IsaQpBcbNaKqSPgtOpySkJJkK0XbbRY7YcDZyCIbdpVfZHmz2sGspQ9wk0-bg8I2QJVJ6QvIP7lo_rysTjAjuMMw-e6rNuBO-zPVZumt9qOelupFhXhuk54gP_kWK3yCLdkwGHBloelbptDJLVK9IyYcPbEIxMLafIiOJ5XVFi5sEuSsvhEu9yi2M2c-BY4hlmxYuZPvGkwmxqFP2X4JQhnYAKhrdk7Hj6A2TnXhWOJu5v9L-JOkaY3MDuE3dQVMX90S_XsXD_Ew6mzUsF3BvY911glHxs\",\"e\":\"AQAB\"}]}"

const tenantWellKnownJwk = "{\"keys\":[{\"use\":\"sig\",\"kty\":\"RSA\",\"kid\":\"30c43677-4c0d-4191-bff1-5ced49059f4f\",\"alg\":\"RS256\",\"n\":\"5E0LW8-pnWJJof9SWVmyOfnijHXeJn7ZUZ0FywAzKjgj5oGJvxMQO0mxa3OTySTkHl0keJecEfxhbHyQOmt_RsGXfjOaKTsVblJwgEyC_LxFn4qCP1KV5m1G_2uSNoImMrBrWXYDwt7cd3Bvk7cUHUrW5YINqpNGv1-BPobq2gCI06mJxESyWH0qJYrCWhJoXUp_pZp4UFa1IzLqK_V0m6kIVg-ad_F6Lzd0MDq2DSRM-iNpQLURGAlvTZKyJEchVG17t4zK9bq21WgF7ses8_zDY_-2xyPPUNRMR76dAHUdnJrBd08XhDuq8ddL_Lg3ZLX7HJBUgpiZuC1W1mf8o39AimnCffXJs73ZtNf3kua9O1SyeE1q3nmB0aXFNvrT_VeTP22bOwI3dIf39esiCTI3HyT_GzbzkXJOAbsCrDgK6tRZlXGJRU80drlfdbI3ZOUHBQiwxwOZ0Pp7NSPD-hPo6YIUoRqcz1heu-uvXQ3cvm0dZhbeuvjYAdxwUL9nofRE65v_8A-wCB1WaG6CBBYHcOr6Prt5ts-W7WKy0nlmRnGCyMzykwj4PTGH0H0PzOxg2IMqQ_gc8ybwT3xgotSUNDCMgMpKfY-1-Vre1cKTFH6Hthx_AWrJeFdEzdvHes75uAX9HC2TQe6BFmUC-GANPif5MitNzDfU_l7-LG8\",\"e\":\"AQAB\"},{\"use\":\"sig\",\"kty\":\"RSA\",\"kid\":\"b45200ad-56d4-4c4e-bc1a-de2181deddbe\",\"alg\":\"RS256\",\"n\":\"uElbd6H4X8uOUBEIS-QrLr-_WfcuvCKJdDCdLGATtiOhtsXEgyYdcaYZbbPOK5Jgf8xEW2qwbvubW7BrVG71zYC9KN_re2pp6_Dcj9qf3h_jOTOv4hLN331uD8c3MXIoHqOV9g7tZaWLL4Cw0tIaSU4h4pMXnYHs0LE9xbi7561DdLXIWPrg7LElbvTkAxpP6aJ8C8ehKDeAIp4QkLE2JXRSHIlwSp4cglVlmujy1ypk4r06YH93aZx9vSXQiNj6sEArllWSe_eqx_B4_dqOTEXJmkVTkzOmOFNpClJ4q8Ih4u1XFV7NQPS-OqIFSKLCuQ-5_yH-Xh1Ny9JhvML4XORJJrs1QC46j7akFr8oH4ttoPpSVmavTP_D2funbEaU6r7k8PTSaa-XfbBS_6PI2Aqj7qzKSBc93tOpg7HIuBo7PRggO-RuGgBK15xpw1VtbxOx8DXnsZnTtyAIxazoilMIsf4QoNF7F-cFH9RsSWy0xq8NbEnfjSjCsofdURsVm4XLpIySfwFC2i08nPX8E6VIOik4MojOkfpWwoJa8eZw3oetTE6sExB17UKYQTgfIhl7q2C1wLaDN-NwwJZRT6d8Hwesy1jasqzQ9GxM0aRPEp4rrcMnUQfc2U05-YWc91-dFk5SrjLmJ5JfJc0q8w_jQdvB6HOGsyhQyNa9gN8\",\"e\":\"AQAB\"}]}"

// AuthenticationTestSuite extends FrameBaseTestSuite for comprehensive authentication testing.
type AuthenticationTestSuite struct {
	tests.BaseTestSuite
	// Mock JWKS server for testing
	mockServer *httptest.Server
	jwksURL    string
}

func initAuthenticationResources(_ context.Context) []definition.TestResource {
	pg := testpostgres.NewWithOpts("frame_test_service",
		definition.WithUserName("ant"), definition.WithPassword("s3cr3t"),
		definition.WithEnableLogging(false), definition.WithUseHostMode(false))

	queue := testnats.NewWithOpts("partition",
		definition.WithUserName("ant"),
		definition.WithPassword("s3cr3t"),
		definition.WithEnableLogging(false))

	hydra := testoryhydra.NewWithOpts(
		testoryhydra.HydraConfiguration, definition.WithDependancies(pg),
		definition.WithEnableLogging(false), definition.WithUseHostMode(true))

	resources := []definition.TestResource{pg, queue, hydra}
	return resources
}

func (s *AuthenticationTestSuite) SetupSuite() {
	if s.InitResourceFunc == nil {
		s.InitResourceFunc = initAuthenticationResources
	}
	s.BaseTestSuite.SetupSuite()
	// Start mock JWKS server
	s.setupMockJWKSServer()
}

func (s *AuthenticationTestSuite) TearDownSuite() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	s.BaseTestSuite.TearDownSuite()
}

func (s *AuthenticationTestSuite) setupMockJWKSServer() {
	s.mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sampleWellKnownJwk))
	}))
	s.jwksURL = s.mockServer.URL + "/.well-known/jwks.json"
}

// TestAuthenticationSuite runs the authentication test suite.
func TestAuthenticationSuite(t *testing.T) {
	suite.Run(t, &AuthenticationTestSuite{})
}

// TestAuthenticationFromContext tests claims extraction from context.
func (s *AuthenticationTestSuite) TestAuthenticationFromContext() {
	testCases := []struct {
		name          string
		setupContext  func(context.Context) context.Context
		expectClaims  bool
		expectedError bool
	}{
		{
			name: "context without claims should return nil",
			setupContext: func(ctx context.Context) context.Context {
				return ctx
			},
			expectClaims:  false,
			expectedError: false,
		},
		{
			name: "context with claims should return claims",
			setupContext: func(ctx context.Context) context.Context {
				claims := &security.AuthenticationClaims{}
				return claims.ClaimsToContext(ctx)
			},
			expectClaims:  true,
			expectedError: false,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := tc.setupContext(t.Context())
				claims := security.ClaimsFromContext(ctx)

				if tc.expectClaims {
					require.NotNil(t, claims, "expected claims to be present in context")
				} else {
					require.Nil(t, claims, "expected no claims in context")
				}
			})
		}
	})
}

// mockJWTConfig extends ConfigurationDefault to return our mock JWKS URL.
type mockJWTConfig struct {
	config.ConfigurationDefault
	jwksURL string
}

func (m *mockJWTConfig) GetOauth2WellKnownJwk() string {
	return m.jwksURL
}

// TestSimpleAuthenticate tests basic JWT authentication.
func (s *AuthenticationTestSuite) TestSimpleAuthenticate() {
	testCases := []struct {
		name         string
		accessKey    string
		audience     string
		issuer       string
		expectError  bool
		expectClaims bool
	}{
		{
			name:         "valid authentication with sample key",
			accessKey:    sampleAccessKey,
			audience:     "",
			issuer:       "",
			expectError:  false,
			expectClaims: true,
		},
		{
			name:         "authentication with specific audience",
			accessKey:    sampleAccessKey,
			audience:     "c2f4j7au6s7f91uqnokg",
			issuer:       "",
			expectError:  false,
			expectClaims: true,
		},
		{
			name:         "authentication with specific issuer",
			accessKey:    sampleAccessKey,
			audience:     "",
			issuer:       "http://127.0.0.1:4444/",
			expectError:  false,
			expectClaims: true,
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			// Create authenticator directly with mock config
			auth := openid.NewJwtTokenAuthenticator(&mockJWTConfig{
				ConfigurationDefault: config.ConfigurationDefault{
					Oauth2JwtVerifyAudience: []string{"c2f4j7au6s7f91uqnokg"},
					Oauth2JwtVerifyIssuer:   "http://127.0.0.1:4444/",
				},
				jwksURL: s.jwksURL,
			})

			ctx := t.Context()
			
			var opts []security.AuthOption
			if tc.audience != "" {
				opts = append(opts, security.WithAudience(tc.audience))
			}
			if tc.issuer != "" {
				opts = append(opts, security.WithIssuer(tc.issuer))
			}
			
			ctx2, err := auth.Authenticate(ctx, tc.accessKey, opts...)

			if tc.expectError {
				require.Error(t, err, "expected authentication to fail")
				return
			}

			require.NoError(t, err, "authentication should succeed")

			if tc.expectClaims {
				claims := security.ClaimsFromContext(ctx2)
				require.NotNil(t, claims, "expected authentication claims in context")
			}
		})
	}
}

// TestSimpleAuthenticateWithOIDC tests OIDC configuration loading.
func (s *AuthenticationTestSuite) TestSimpleAuthenticateWithOIDC() {
	testCases := []struct {
		name        string
		serviceURI  string
		expectError bool
	}{
		{
			name:        "OIDC configuration with Google",
			serviceURI:  "https://accounts.google.com",
			expectError: false,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv("OAUTH2_SERVICE_URI", tc.serviceURI)

				ctx := t.Context()
				cfg, err := config.LoadWithOIDC[config.ConfigurationDefault](ctx)

				if tc.expectError {
					require.Error(t, err, "expected OIDC configuration to fail")
					return
				}

				require.NoError(t, err, "OIDC configuration should load successfully")
				require.NotEmpty(t, cfg.GetOauth2UserInfoEndpoint(), "user info endpoint should be set")
				require.NotEmpty(t, cfg.GetOauth2WellKnownJwkData(), "JWK data should be loaded")

				t.Logf("Configuration UserInfo: %s", cfg.GetOauth2UserInfoEndpoint())
				t.Logf("Configuration JWK: %s", cfg.GetOauth2WellKnownJwkData())
			})
		}
	})
}

// TestTenancySkipFunctions tests the tenancy check skip functionality.
func (s *AuthenticationTestSuite) TestTenancySkipFunctions() {
	testCases := []struct {
		name        string
		setup       func(context.Context, map[string]string) (context.Context, map[string]string)
		checkResult func(*testing.T, context.Context, map[string]string)
	}{
		{
			name: "SkipTenancyChecksFromMap with skip_tenancy_check=true should set skip flag",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				m["skip_tenancy_check"] = "true"
				ctx = security.SkipTenancyChecksFromMap(ctx, m)
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.True(t, security.IsTenancyChecksOnClaimSkipped(ctx), "tenancy checks should be skipped")
			},
		},
		{
			name: "SkipTenancyChecksFromMap with skip_tenancy_check=false should not set skip flag",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				m["skip_tenancy_check"] = "false"
				ctx = security.SkipTenancyChecksFromMap(ctx, m)
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.False(t, security.IsTenancyChecksOnClaimSkipped(ctx), "tenancy checks should not be skipped")
			},
		},
		{
			name: "SkipTenancyChecksFromMap without skip_tenancy_check should not set skip flag",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				ctx = security.SkipTenancyChecksFromMap(ctx, m)
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.False(t, security.IsTenancyChecksOnClaimSkipped(ctx), "tenancy checks should not be skipped")
			},
		},
		{
			name: "SkipTenancyChecksFromMap with other values should not set skip flag",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				m["skip_tenancy_check"] = "maybe"
				m["other_key"] = "value"
				ctx = security.SkipTenancyChecksFromMap(ctx, m)
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.False(t, security.IsTenancyChecksOnClaimSkipped(ctx), "tenancy checks should not be skipped")
			},
		},
		{
			name: "SkipTenancyChecksToMap with skip flag should not set skip_tenancy_check in map",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				ctx = security.SkipTenancyChecksOnClaims(ctx)
				m = security.SkipTenancyChecksToMap(ctx, m)
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.True(t, security.IsTenancyChecksOnClaimSkipped(ctx), "tenancy checks should be skipped")
				require.NotContains(t, m, "skip_tenancy_check", "map should not contain skip_tenancy_check key")
			},
		},
		{
			name: "SkipTenancyChecksToMap without skip flag should set skip_tenancy_check=true in map",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				m = security.SkipTenancyChecksToMap(ctx, m)
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.False(t, security.IsTenancyChecksOnClaimSkipped(ctx), "tenancy checks should not be skipped")
				require.Equal(t, "true", m["skip_tenancy_check"], "map should contain skip_tenancy_check=true")
			},
		},
		{
			name: "SkipTenancyChecksToMap preserves existing map values",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				m["existing_key"] = "existing_value"
				m = security.SkipTenancyChecksToMap(ctx, m)
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.Equal(t, "existing_value", m["existing_key"], "existing map values should be preserved")
				require.Equal(t, "true", m["skip_tenancy_check"], "skip_tenancy_check should be set to true")
			},
		},
		{
			name: "IsTenancyChecksOnClaimSkipped with skip flag returns true",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				ctx = security.SkipTenancyChecksOnClaims(ctx)
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.True(t, security.IsTenancyChecksOnClaimSkipped(ctx), "should return true when skip flag is set")
			},
		},
		{
			name: "IsTenancyChecksOnClaimSkipped without skip flag returns false",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.False(t, security.IsTenancyChecksOnClaimSkipped(ctx), "should return false when skip flag is not set")
			},
		},
		{
			name: "IsTenancyChecksOnClaimSkipped with wrong context value type returns false",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				ctx = context.WithValue(ctx, "wrong_key", "string_instead_of_bool")
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.False(t, security.IsTenancyChecksOnClaimSkipped(ctx), "should return false for wrong value type")
			},
		},
		{
			name: "SkipTenancyChecksOnClaims sets skip flag correctly",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				ctx = security.SkipTenancyChecksOnClaims(ctx)
				return ctx, m
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.True(t, security.IsTenancyChecksOnClaimSkipped(ctx), "SkipTenancyChecksOnClaims should set skip flag")
			},
		},
		{
			name: "Round trip: FromMap -> ToMap preserves skip state",
			setup: func(ctx context.Context, m map[string]string) (context.Context, map[string]string) {
				m["skip_tenancy_check"] = "true"
				ctx = security.SkipTenancyChecksFromMap(ctx, m)
				// Clear the original map and recreate it
				newMap := make(map[string]string)
				newMap = security.SkipTenancyChecksToMap(ctx, newMap)
				return ctx, newMap
			},
			checkResult: func(t *testing.T, ctx context.Context, m map[string]string) {
				require.True(t, security.IsTenancyChecksOnClaimSkipped(ctx), "context should have skip flag")
				require.NotContains(t, m, "skip_tenancy_check", "map should not contain skip_tenancy_check when context has skip flag")
			},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			m := make(map[string]string)
			ctx, m = tc.setup(ctx, m)
			tc.checkResult(t, ctx, m)
		})
	}
}
