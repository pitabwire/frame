package frameauthorization

import (
	"context"
	"testing"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/stretchr/testify/suite"
)

// AuthorizerTestSuite extends FrameBaseTestSuite for authorization testing with real dependencies
type AuthorizerTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestAuthorizerCreation tests authorizer creation with real dependencies
func (s *AuthorizerTestSuite) TestAuthorizerCreation() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "authorizer_creation_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "authorizer-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("AuthorizerCreation", func(t *testing.T) {
			// Test authorizer creation
			authorizer := NewAuthorizer(nil, nil, nil)
			s.NotNil(authorizer, "Should create authorizer")
		})

		t.Run("AuthorizerIsEnabled", func(t *testing.T) {
			// Test authorizer enabled state
			authorizer := NewAuthorizer(nil, nil, nil)
			enabled := authorizer.IsEnabled()
			s.False(enabled, "Should be disabled with nil config")
		})
	})
}

// TestAuthorizerAccess tests authorization access control with real dependencies
func (s *AuthorizerTestSuite) TestAuthorizerAccess() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "authorizer_access_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "authorizer-access-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("HasAccessWithDisabledAuthorizer", func(t *testing.T) {
			// Test access check with disabled authorizer
			authorizer := NewAuthorizer(nil, nil, nil)
			hasAccess, err := authorizer.HasAccess(ctx, "read", "user123")
			s.True(hasAccess, "Should allow access when authorizer is disabled")
			s.NoError(err, "Should not return error when authorizer is disabled")
		})

		t.Run("AuthorizerInterfaceCompliance", func(t *testing.T) {
			// Test that our authorizer implements the interface correctly
			var _ Authorizer = NewAuthorizer(nil, nil, nil)
		})
	})
}

func TestAuthorizerTestSuite(t *testing.T) {
	suite.Run(t, &AuthorizerTestSuite{
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
