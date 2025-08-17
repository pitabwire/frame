package frameserver

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

// ServerTestSuite extends FrameBaseTestSuite for server testing with real dependencies
type ServerTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestServerManagerCreation tests server manager creation with real dependencies
func (s *ServerTestSuite) TestServerManagerCreation() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "server_creation_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "server-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("ServerManagerCreation", func(t *testing.T) {
			// Test server manager creation
			manager := NewServerManager(nil, nil, nil)
			s.NotNil(manager, "Should create server manager")
		})

		t.Run("ServerManagerInterfaceCompliance", func(t *testing.T) {
			// Test that our server manager implements the interface correctly
			var _ ServerManager = NewServerManager(nil, nil, nil)
		})
	})
}

// TestServerManagerOperations tests server manager operations with real dependencies
func (s *ServerTestSuite) TestServerManagerOperations() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "server_operations_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "server-operations-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("ServerManagerState", func(t *testing.T) {
			manager := NewServerManager(nil, nil, nil)
			
			s.False(manager.IsRunning(), "Should not be running initially")
			s.False(manager.IsHealthy(ctx), "Should not be healthy initially")
		})

		t.Run("ServerManagerStartStop", func(t *testing.T) {
			manager := NewServerManager(nil, nil, nil)
			
			// Test stop without start
			err := manager.Stop(ctx)
			s.NoError(err, "Should stop successfully even without start")
		})
	})
}

func TestServerTestSuite(t *testing.T) {
	suite.Run(t, &ServerTestSuite{
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
