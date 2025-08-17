package framequeue

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

// ManagerTestSuite extends FrameBaseTestSuite for manager testing with real dependencies
type ManagerTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestManagerCreation tests manager creation with real dependencies
func (s *ManagerTestSuite) TestManagerCreation() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "manager_creation_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "manager-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("ManagerCreation", func(t *testing.T) {
			// Test manager creation
			manager := NewQueueManager(nil, nil)
			s.NotNil(manager, "Should create manager")
		})

		t.Run("ManagerInterfaceCompliance", func(t *testing.T) {
			// Test that our manager implements the interface correctly
			var _ QueueManager = NewQueueManager(nil, nil)
		})
	})
}

func TestManagerTestSuite(t *testing.T) {
	suite.Run(t, &ManagerTestSuite{
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
