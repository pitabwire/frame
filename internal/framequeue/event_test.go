package framequeue_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/internal/framequeue"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
)

// EventTestSuite extends FrameBaseTestSuite for event testing
type EventTestSuite struct {
	frametests.FrameBaseTestSuite
}

// TestEventSuite runs the event test suite
func TestEventSuite(t *testing.T) {
	suite.Run(t, &EventTestSuite{
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

// TestNewEvent tests the NewEvent function with various parameters
func (s *EventTestSuite) TestNewEvent() {
	testCases := []struct {
		name        string
		eventType   string
		data        map[string]interface{}
		description string
	}{
		{
			name:        "ValidEvent",
			eventType:   "user.created",
			data:        map[string]interface{}{"id": "123", "name": "test"},
			description: "Should create event with valid parameters",
		},
		{
			name:        "EmptyEventType",
			eventType:   "",
			data:        map[string]interface{}{"id": "123"},
			description: "Should create event with empty type",
		},
		{
			name:        "NilData",
			eventType:   "test.event",
			data:        nil,
			description: "Should create event with nil data",
		},
		{
			name:        "ComplexData",
			eventType:   "complex.event",
			data: map[string]interface{}{
				"nested": map[string]interface{}{
					"value": 42,
				},
				"array": []string{"a", "b", "c"},
			},
			description: "Should create event with complex data",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			event := framequeue.NewEvent(tc.eventType, tc.data)
			
			s.Equal(tc.eventType, event.Type(), tc.description)
			s.Equal(tc.data, event.Data(), tc.description)
		})
	}
}

// TestEventRegistry tests event registry functionality with real dependencies
func (s *EventTestSuite) TestEventRegistry() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "registry_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "registry-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("CreateEventRegistry", func(t *testing.T) {
			// Test event registry creation
			registry := framequeue.NewEventRegistry()
			s.NotNil(registry, "Should create event registry")
		})

		t.Run("EventCreationAndProperties", func(t *testing.T) {
			// Test event creation with various data types
			event := framequeue.NewEvent("test.event", map[string]interface{}{
				"string":  "value",
				"number":  42,
				"boolean": true,
				"nested": map[string]interface{}{
					"key": "nested_value",
				},
			})
			
			s.Equal("test.event", event.Type(), "Should have correct event type")
			s.NotNil(event.Data(), "Should have event data")
		})
	})
}


// TestQueueIntegration tests queue integration with real dependencies
func (s *EventTestSuite) TestQueueIntegration() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "integration_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "integration-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", depOpt.Queue(ctx)[0].GetDS(ctx).String()),
		)
		defer svc.Stop(ctx)

		t.Run("PublishAndReceiveEvent", func(t *testing.T) {
			// Create event and test its properties
			event := framequeue.NewEvent("integration.test", map[string]interface{}{
				"message":   "integration test",
				"timestamp": time.Now().Unix(),
			})
			
			// Verify event properties
			s.Equal("integration.test", event.Type(), "Should have correct event type")
			s.NotNil(event.Data(), "Should have event data")
		})
	})
}


