package framedata

import (
	"context"
	"testing"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/stretchr/testify/suite"
)

// DatastoreTestSuite extends FrameBaseTestSuite for datastore testing with real dependencies
type DatastoreTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestDatastoreCreation tests datastore manager creation with real dependencies
func (s *DatastoreTestSuite) TestDatastoreCreation() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "datastore_creation_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "datastore-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("DatastoreManagerCreation", func(t *testing.T) {
			// Test datastore manager creation
			manager := NewDatastoreManager(nil, nil, nil)
			s.NotNil(manager, "Should create datastore manager")
		})

		t.Run("DatastoreManagerInitialization", func(t *testing.T) {
			// Test datastore manager initialization
			manager := NewDatastoreManager(nil, nil, nil)
			err := manager.Initialize(ctx)
			// With nil config, initialization should fail gracefully
			s.Error(err, "Should fail initialization with nil config")
		})
	})
}

// TestDatastoreOperations tests datastore operations with real dependencies
func (s *DatastoreTestSuite) TestDatastoreOperations() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "datastore_operations_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "datastore-operations-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("DatastoreInterfaceCompliance", func(t *testing.T) {
			// Test that our datastore manager implements the interface correctly
			var _ DatastoreManager = NewDatastoreManager(nil, nil, nil)
		})

		t.Run("DatastoreHealthCheck", func(t *testing.T) {
			// Test datastore health check
			manager := NewDatastoreManager(nil, nil, nil)
			healthy := manager.IsHealthy(ctx)
			s.False(healthy, "Should not be healthy with nil config")
		})
	})
}

func TestDatastoreTestSuite(t *testing.T) {
	suite.Run(t, new(DatastoreTestSuite))
}
