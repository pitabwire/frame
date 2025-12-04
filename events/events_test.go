package events_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tests"
)

// EventsTestSuite extends BaseTestSuite for comprehensive events testing.
type EventsTestSuite struct {
	tests.BaseTestSuite
}

type MessageToTest struct {
	Service *frame.Service
	Count   int
}

func (event *MessageToTest) Name() string {
	return "message.to.test"
}

func (event *MessageToTest) PayloadType() any {
	return new(string)
}

func (event *MessageToTest) Validate(_ context.Context, payload any) error {
	if _, ok := payload.(*string); !ok {
		return fmt.Errorf("payload is %T not of type %T", payload, event.PayloadType())
	}
	return nil
}

func (event *MessageToTest) Execute(ctx context.Context, payload any) error {
	m, _ := payload.(*string)
	message := *m
	logger := event.Service.Log(ctx).WithField("payload", message).WithField("type", event.Name())
	logger.Info("handling event")
	event.Count++
	return nil
}

// TestEventsSuite runs the events test suite.
func TestEventsSuite(t *testing.T) {
	suite.Run(t, &EventsTestSuite{})
}

// TestServiceRegisterEventsWorks tests event registration and subscription.
func (s *EventsTestSuite) TestServiceRegisterEventsWorks() {
	testCases := []struct {
		name        string
		serviceName string
		queueName   string
	}{
		{
			name:        "register events with queue subscription",
			serviceName: "Test Srv",
			queueName:   "events_queue",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				queue := dep.ByIsQueue(t.Context())

				cfg, err := config.FromEnv[config.ConfigurationDefault]()
				require.NoError(t, err, "configuration loading should succeed")

				if queue != nil {
					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						cfg.EventsQueueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", "svc.frame.internal._queue").
							ExtendQuery("stream_name", "svc_frame").
							ExtendQuery("stream_subjects", "svc.frame.>").
							ExtendQuery("consumer_durable_name", "svc_frame_internal_queue").
							ExtendQuery("consumer_filter_subject", "svc.frame.internal._queue").
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						cfg.EventsQueueURL = qDS.ExtendPath("svc.frame.internal._queue").String()
					}
				}
				cfg.EventsQueueName = tc.queueName
				events := frame.WithRegisterEvents(&MessageToTest{})
				ctx, svc := frame.NewService(
					frame.WithName(tc.serviceName),
					events,
					frame.WithConfig(&cfg),
					frametests.WithNoopDriver(),
				)

				qm := svc.QueueManager()

				subs, _ := qm.GetSubscriber(cfg.EventsQueueName)
				if subs != nil && subs.Initiated() {
					t.Fatalf("Subscription to event queue is invalid")
				}

				err = svc.Run(ctx, "")
				require.NoError(t, err, "service should run without error")

				subs, _ = qm.GetSubscriber(cfg.EventsQueueName)
				require.True(t, subs.Initiated(), "subscription to event queue should be initiated")

				svc.Stop(ctx)
			})
		}
	})
}

// TestServiceEventsPublishingWorks tests event publishing and handling.
func (s *EventsTestSuite) TestServiceEventsPublishingWorks() {
	testCases := []struct {
		name          string
		serviceName   string
		payload       string
		initialCount  int
		expectedCount int
	}{
		{
			name:          "publish and handle event successfully",
			serviceName:   "Test Srv",
			payload:       "test message",
			initialCount:  50,
			expectedCount: 51,
		},
		{
			name:          "publish event with empty payload",
			serviceName:   "Test Srv",
			payload:       "",
			initialCount:  0,
			expectedCount: 1,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				cfg, err := config.FromEnv[config.ConfigurationDefault]()
				require.NoError(t, err, "configuration loading should succeed")

				// Set queue connection from dependency
				if queue != nil {
					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						cfg.EventsQueueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", "svc.frame.internal._queue").
							ExtendQuery("stream_name", "svc_frame").
							ExtendQuery("stream_subjects", "svc.frame.>").
							ExtendQuery("consumer_durable_name", "svc_frame_internal_queue").
							ExtendQuery("consumer_filter_subject", "svc.frame.internal._queue").
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						cfg.EventsQueueURL = qDS.ExtendPath("svc.frame.internal._queue").String()
					}
				}

				ctx, svc := frame.NewService(
					frame.WithName(tc.serviceName),
					frame.WithConfig(&cfg),
					frametests.WithNoopDriver(),
				)

				testEvent := MessageToTest{Service: svc, Count: tc.initialCount}
				events := frame.WithRegisterEvents(&testEvent)

				svc.Init(ctx, events)
				err = svc.Run(ctx, "")
				require.NoError(t, err, "service should run without error")

				evtMan := svc.EventsManager()
				err = evtMan.Emit(ctx, testEvent.Name(), tc.payload)
				require.NoError(t, err, "event emission should succeed")

				if len(tc.payload) > 0 {
					time.Sleep(2 * time.Second)
					require.Equal(
						t,
						tc.expectedCount,
						testEvent.Count,
						"event should be processed and count incremented",
					)
				}

				svc.Stop(ctx)
			})
		}
	})
}
