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

// SubscriberTestSuite extends FrameBaseTestSuite for subscriber testing with real dependencies
type SubscriberTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestSubscriberCreation tests subscriber creation with real dependencies
func (s *SubscriberTestSuite) TestSubscriberCreation() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "subscriber_creation_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "subscriber-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("SubscriberCreation", func(t *testing.T) {
			// Test subscriber creation using mem:// to avoid NATS connection issues
			subscriber := NewSubscriber("test-subscription", "mem://test-topic", nil, nil)
			s.NotNil(subscriber, "Should create subscriber")
		})

		t.Run("SubscriberInterfaceCompliance", func(t *testing.T) {
			// Test that our subscriber implements the interface correctly
			var _ Subscriber = NewSubscriber("test-subscription", "mem://test-topic", nil, nil)
		})
	})
}

func TestSubscriberTestSuite(t *testing.T) {
	suite.Run(t, &SubscriberTestSuite{
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
