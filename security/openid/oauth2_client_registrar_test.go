package openid_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testoryhydra"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/pitabwire/frame/tests"
)

// JwtTestSuite extends BaseTestSuite for comprehensive JWT testing.
type JwtTestSuite struct {
	tests.BaseTestSuite
}

func initJwtResources(_ context.Context) []definition.TestResource {
	pg := testpostgres.NewWithOpts("frame_test_service",
		definition.WithUserName("ant"), definition.WithCredential("s3cr3t"),
		definition.WithEnableLogging(false), definition.WithUseHostMode(false))

	queue := testnats.NewWithOpts("partition",
		definition.WithUserName("ant"),
		definition.WithCredential("s3cr3t"),
		definition.WithEnableLogging(false))

	hydra := testoryhydra.NewWithOpts(
		testoryhydra.HydraConfiguration, definition.WithDependancies(pg),
		definition.WithEnableLogging(false), definition.WithUseHostMode(true))

	resources := []definition.TestResource{pg, queue, hydra}
	return resources
}

func (s *JwtTestSuite) SetupSuite() {
	if s.InitResourceFunc == nil {
		s.InitResourceFunc = initJwtResources
	}
	s.BaseTestSuite.SetupSuite()
}

// TestJwtSuite runs the JWT test suite.
func TestJwtSuite(t *testing.T) {
	suite.Run(t, &JwtTestSuite{})
}

// TestServiceRegisterForJwtWithParams tests JWT registration and unregistration.
func (s *JwtTestSuite) TestServiceRegisterForJwtWithParams() {
	testCases := []struct {
		name         string
		serviceName  string
		clientName   string
		clientID     string
		clientSecret string
		expectError  bool
	}{
		{
			name:         "register for JWT with valid parameters",
			serviceName:  "Test Srv",
			clientName:   "Testing CLI",
			clientID:     "test-cli-dev",
			clientSecret: "topS3cret",
			expectError:  false,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Skip this test as it requires external OAuth2 service
				t.Skip("Only run this test manually by removing the skip - requires external OAuth2 service")

				ctx := t.Context()
				hydra := dep.ByImageName(testoryhydra.OryHydraImage)

				ctx, svc := frame.NewService(
					frame.WithName(tc.serviceName),
					frame.WithConfig(&config.ConfigurationDefault{
						Oauth2ServiceAdminURI: hydra.GetDS(ctx).String(),
					}),
				)

				sm := svc.SecurityManager()
				clientRegistrar := sm.GetOauth2ClientRegistrar(ctx)

				response, err := clientRegistrar.RegisterForJwtWithParams(
					ctx, hydra.GetDS(ctx).String(), tc.clientName, tc.clientID, tc.clientSecret,
					"", []string{}, map[string]string{})

				if tc.expectError {
					require.Error(t, err, "JWT registration should fail")
					return
				}

				require.NoError(t, err, "JWT registration should succeed")
				require.NotEmpty(t, response, "JWT registration response should not be empty")

				sm.SetJwtClient(tc.clientID, tc.clientSecret, response)
				svc.Log(ctx).WithField("client id", response).Info("successfully registered for JWT")

				err = clientRegistrar.UnRegisterForJwt(ctx, hydra.GetDS(ctx).String(), sm.JwtClientID())
				require.NoError(t, err, "JWT unregistration should succeed")
			})
		}
	})
}
