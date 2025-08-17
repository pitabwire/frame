package frameauth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/internal/frameauth"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
)

// AuthenticatorTestSuite extends FrameBaseTestSuite for authentication testing
type AuthenticatorTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestAuthenticatorSuite runs the authenticator test suite
func TestAuthenticatorSuite(t *testing.T) {
	suite.Run(t, &AuthenticatorTestSuite{
		FrameBaseTestSuite: frametests.FrameBaseTestSuite{
			InitResourceFunc: func(_ context.Context) []definition.TestResource {
				return []definition.TestResource{
					testpostgres.New(),
					testnats.New(),
				}
			},
		},
	})
}

// TestNewAuthenticator tests the NewAuthenticator function with real dependencies
func (s *AuthenticatorTestSuite) TestNewAuthenticator() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "auth_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "auth-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("CreateAuthenticator", func(t *testing.T) {
			// Test authenticator creation
			authenticator := frameauth.NewAuthenticator(nil, nil)
			s.NotNil(authenticator, "Should create authenticator")
		})

		t.Run("AuthenticatorIsEnabled", func(t *testing.T) {
			// Test authenticator enabled state
			authenticator := frameauth.NewAuthenticator(nil, nil)
			enabled := authenticator.IsEnabled()
			s.False(enabled, "Should be disabled with nil config")
		})
	})
}

// TestAuthenticatorHTTPMiddleware tests HTTP middleware functionality with real dependencies
func (s *AuthenticatorTestSuite) TestAuthenticatorHTTPMiddleware() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "middleware_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "middleware-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("HTTPMiddlewareCreation", func(t *testing.T) {
			// Test HTTP middleware creation
			authenticator := frameauth.NewAuthenticator(nil, nil)
			middleware := authenticator.HTTPMiddleware("test-audience", "test-issuer")
			s.NotNil(middleware, "Should create HTTP middleware")
		})

		t.Run("GRPCInterceptorCreation", func(t *testing.T) {
			// Test gRPC interceptor creation
			authenticator := frameauth.NewAuthenticator(nil, nil)
			unaryInterceptor := authenticator.UnaryInterceptor("test-audience", "test-issuer")
			streamInterceptor := authenticator.StreamInterceptor("test-audience", "test-issuer")
			s.NotNil(unaryInterceptor, "Should create unary interceptor")
			s.NotNil(streamInterceptor, "Should create stream interceptor")
		})
	})
}



