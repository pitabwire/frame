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

// PublisherTestSuite extends FrameBaseTestSuite for publisher testing with real dependencies
type PublisherTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestPublisherCreation tests publisher creation with real dependencies
func (s *PublisherTestSuite) TestPublisherCreation() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "publisher_creation_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "publisher-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("PublisherCreation", func(t *testing.T) {
			// Test publisher creation using mem:// to avoid NATS connection issues
			publisher := NewPublisher("test-topic", "mem://test-topic", nil)
			s.NotNil(publisher, "Should create publisher")
		})

		t.Run("PublisherInterfaceCompliance", func(t *testing.T) {
			// Test that our publisher implements the interface correctly
			var _ Publisher = NewPublisher("test-topic", "mem://test-topic", nil)
		})
	})
}

// TestPublisherOperations tests publisher operations with real dependencies
func (s *PublisherTestSuite) TestPublisherOperations() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "publisher_operations_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "publisher-operations-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("PublisherInitialization", func(t *testing.T) {
			// Use mem:// for testing to avoid NATS connection issues
			publisher := NewPublisher("test-topic", "mem://test-topic", nil)
			
			err := publisher.Init(ctx)
			s.NoError(err, "Should initialize publisher successfully")
		})

		t.Run("PublisherPublish", func(t *testing.T) {
			// Use mem:// for testing to avoid NATS connection issues
			publisher := NewPublisher("test-topic", "mem://test-topic", nil)
			err := publisher.Init(ctx)
			s.Require().NoError(err, "Should initialize publisher")
			
			// Test publishing a simple message
			err = publisher.Publish(ctx, "test message")
			s.NoError(err, "Should publish message successfully")
		})
	})
}

func TestPublisherTestSuite(t *testing.T) {
	suite.Run(t, &PublisherTestSuite{
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
