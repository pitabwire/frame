package queue_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/tests"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// QueueTestSuite extends BaseTestSuite for comprehensive queueManager testing.
type QueueTestSuite struct {
	tests.BaseTestSuite
}

// TestQueueSuite runs the queueManager test suite.
func TestQueueSuite(t *testing.T) {
	suite.Run(t, &QueueTestSuite{})
}

// TestServiceRegisterPublisherNotSet tests publishing when no publisher is registered.
func (s *QueueTestSuite) TestServiceRegisterPublisherNotSet() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		message     []byte
	}{
		{
			name:        "publish to unregistered topic",
			serviceName: "Test Srv",
			topic:       "random",
			message:     []byte(""),
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName))

				qm := srv.QueueManager()

				err := qm.Publish(ctx, tc.topic, tc.message)
				require.Error(t, err, "Publishing to unregistered topic should fail")
			})
		}
	})
}

// TestServiceRegisterPublisherNotInitialized tests publishing when publisher is registered but not initialized.
func (s *QueueTestSuite) TestServiceRegisterPublisherNotInitialized() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
		message     []byte
	}{
		{
			name:        "publish to uninitialized publisher",
			serviceName: "Test Srv",
			topic:       "random",
			queueURL:    "mem://topicA",
			message:     []byte(""),
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error

				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				queueURL := tc.queueURL
				if queue != nil {
					queueSubject := strings.Replace(queueURL, "mem://", "", 1) + dep.Prefix()

					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						queueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", queueSubject).
							ExtendQuery("stream_name", queueSubject).
							ExtendQuery("stream_subjects", queueSubject).
							ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
							ExtendQuery("consumer_filter_subject", queueSubject).
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						queueURL = qDS.ExtendPath(queueSubject).String()
					}
				}

				opt := frame.WithRegisterPublisher("test", queueURL)
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), opt)

				qm := srv.QueueManager()

				err = qm.Publish(ctx, tc.topic, tc.message)
				require.Error(t, err, "Publishing to uninitialized publisher should fail")
			})
		}
	})
}

// TestServiceRegisterPublisher tests publishing with registered publisher.
func (s *QueueTestSuite) TestServiceRegisterPublisher() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
		message     []byte
	}{
		{
			name:        "publish to registered topic",
			serviceName: "Test Srv",
			topic:       "test",
			queueURL:    "mem://topicA",
			message:     []byte("test message"),
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error

				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				queueURL := tc.queueURL
				if queue != nil {
					queueSubject := strings.Replace(queueURL, "mem://", "", 1) + dep.Prefix()

					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						queueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", queueSubject).
							ExtendQuery("stream_name", queueSubject).
							ExtendQuery("stream_subjects", queueSubject).
							ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
							ExtendQuery("consumer_filter_subject", queueSubject).
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						queueURL = qDS.ExtendPath(queueSubject).String()
					}
				}

				opt := frame.WithRegisterPublisher(tc.topic, queueURL)
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), opt, frametests.WithNoopDriver())

				err = srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				err = qm.Publish(ctx, tc.topic, tc.message)
				require.NoError(t, err, "Publishing to registered topic should succeed")

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceRegisterPublisherMultiple tests multiple publishers.
//
//nolint:gocognit
func (s *QueueTestSuite) TestServiceRegisterPublisherMultiple() {
	testCases := []struct {
		name         string
		serviceName  string
		topics       []string
		queueURLs    []string
		testMessages map[string][]byte
		invalidTopic string
	}{
		{
			name:        "multiple publishers",
			serviceName: "Test Srv",
			topics:      []string{"test-multiple-publisher", "test-multiple-publisher-2"},
			queueURLs:   []string{"mem://topicA", "mem://topicB"},
			testMessages: map[string][]byte{
				"test-multiple-publisher":   []byte("Testament"),
				"test-multiple-publisher-2": []byte("Testament"),
			},
			invalidTopic: "test-multiple-3",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error
				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				opts := []frame.Option{frame.WithName(tc.serviceName), frametests.WithNoopDriver()}
				for i, topic := range tc.topics {
					queueURL := tc.queueURLs[i]
					if queue != nil {
						queueSubject := strings.Replace(queueURL, "mem://", "", 1) + dep.Prefix()

						qDS := queue.GetDS(ctx)
						if qDS.IsNats() {
							qDS, err = qDS.WithUser("ant")
							require.NoError(t, err)

							qDS, err = qDS.WithPassword("s3cr3t")
							require.NoError(t, err)

							queueURL = qDS.
								ExtendQuery("jetstream", "true").
								ExtendQuery("subject", queueSubject).
								ExtendQuery("stream_name", queueSubject).
								ExtendQuery("stream_subjects", queueSubject).
								ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
								ExtendQuery("consumer_filter_subject", queueSubject).
								ExtendQuery("consumer_ack_policy", "explicit").
								ExtendQuery("consumer_deliver_policy", "all").
								ExtendQuery("consumer_replay_policy", "instant").
								ExtendQuery("stream_retention", "workqueue").
								ExtendQuery("stream_storage", "file").
								String()
						} else {
							queueURL = qDS.ExtendPath(queueSubject).String()
						}
					}

					opts = append(opts, frame.WithRegisterPublisher(topic, queueURL))
				}

				ctx, srv := frame.NewService(opts...)

				err = srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				// Test publishing to valid topics
				for topic, message := range tc.testMessages {
					err = qm.Publish(ctx, topic, message)
					require.NoError(t, err, "Publishing to registered topic %s should succeed", topic)
				}

				// Test publishing to invalid topic
				err = qm.Publish(ctx, tc.invalidTopic, []byte("Testament"))
				require.Error(t, err, "Publishing to unregistered topic should fail")

				srv.Stop(ctx)
			})
		}
	})
}

type msgHandler struct {
	f func(ctx context.Context, metadata map[string]string, message []byte) error
}

func (h *msgHandler) Handle(ctx context.Context, metadata map[string]string, message []byte) error {
	return h.f(ctx, metadata, message)
}

type handlerWithError struct{}

func (h *handlerWithError) Handle(_ context.Context, _ map[string]string, _ []byte) error {
	return errors.New("handler error")
}

// TestServiceRegisterSubscriber tests subscriber registration.
func (s *QueueTestSuite) TestServiceRegisterSubscriber() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
		handler     func(ctx context.Context, metadata map[string]string, message []byte) error
	}{
		{
			name:        "register subscriber with handler",
			serviceName: "Test Srv",
			topic:       "test-subscriber",
			queueURL:    "mem://topicA",
			handler: func(_ context.Context, _ map[string]string, _ []byte) error {
				return nil
			},
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error

				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				queueURL := tc.queueURL
				if queue != nil {
					queueSubject := strings.Replace(queueURL, "mem://", "", 1) + dep.Prefix()

					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						queueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", queueSubject).
							ExtendQuery("stream_name", queueSubject).
							ExtendQuery("stream_subjects", queueSubject).
							ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
							ExtendQuery("consumer_filter_subject", queueSubject).
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						queueURL = qDS.ExtendPath(queueSubject).String()
					}
				}

				handler := &msgHandler{f: tc.handler}
				opt := frame.WithRegisterSubscriber(tc.topic, queueURL, handler)
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), opt, frametests.WithNoopDriver())

				err = srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				subscriber, err := qm.GetSubscriber(tc.topic)
				require.NoError(t, err, "Subscriber should exist")
				require.NotNil(t, subscriber, "Subscriber should be registered")
				require.True(t, subscriber.Initiated(), "Subscriber should be initiated")

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceRegisterSubscriberValidateMessages tests message validation.
func (s *QueueTestSuite) TestServiceRegisterSubscriberValidateMessages() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
		messages    []string
		expectCount int
	}{
		{
			name:        "validate multiple messages",
			serviceName: "Test Srv",
			topic:       "test-validate",
			queueURL:    "mem://topicA",
			messages:    []string{"message1", "message2", "message3"},
			expectCount: 3,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error

				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				queueURL := tc.queueURL
				if queue != nil {
					queueSubject := strings.Replace(queueURL, "mem://", "", 1) + dep.Prefix()

					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						queueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", queueSubject).
							ExtendQuery("stream_name", queueSubject).
							ExtendQuery("stream_subjects", queueSubject).
							ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
							ExtendQuery("consumer_filter_subject", queueSubject).
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						queueURL = qDS.ExtendPath(queueSubject).String()
					}
				}

				var receivedCount int
				var mu sync.Mutex

				handler := &msgHandler{
					f: func(_ context.Context, _ map[string]string, _ []byte) error {
						mu.Lock()
						receivedCount++
						mu.Unlock()
						return nil
					},
				}

				opt := frame.WithRegisterSubscriber(tc.topic, queueURL, handler)
				pubOpt := frame.WithRegisterPublisher(tc.topic, queueURL)
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), opt, pubOpt, frametests.WithNoopDriver())

				err = srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				// Publish messages
				for _, msg := range tc.messages {
					err = qm.Publish(ctx, tc.topic, []byte(msg))
					require.NoError(t, err, "Publishing message should succeed")
				}

				// Wait for message processing
				time.Sleep(2 * time.Second)

				mu.Lock()
				count := receivedCount
				mu.Unlock()
				require.Equal(t, tc.expectCount, count, "Should receive expected number of messages")

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceSubscriberValidateJetstreamMessages tests Jetstream message validation.
//
//nolint:gocognit
func (s *QueueTestSuite) TestServiceSubscriberValidateJetstreamMessages() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		messages    []map[string]any
		expectCount int
	}{
		{
			name:        "validate Jetstream messages",
			serviceName: "Test Srv",
			topic:       "test-jetstream",
			messages: []map[string]any{
				{"id": 1, "data": "test1"},
				{"id": 2, "data": "test2"},
			},
			expectCount: 2,
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error

				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				queueURL := "mem://" + tc.topic
				if queue != nil {
					queueSubject := tc.topic

					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						queueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", queueSubject).
							ExtendQuery("stream_name", queueSubject).
							ExtendQuery("stream_subjects", queueSubject).
							ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
							ExtendQuery("consumer_filter_subject", queueSubject).
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						queueURL = qDS.ExtendPath(queueSubject).String()
					}
				}

				var receivedCount int
				var mu sync.Mutex

				handler := &msgHandler{
					f: func(_ context.Context, _ map[string]string, message []byte) error {
						var data map[string]any
						if err = json.Unmarshal(message, &data); err != nil {
							return err
						}
						mu.Lock()
						receivedCount++
						mu.Unlock()
						return nil
					},
				}

				opt := frame.WithRegisterSubscriber(tc.topic, queueURL, handler)
				pubOpt := frame.WithRegisterPublisher(tc.topic, queueURL)
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), opt, pubOpt, frametests.WithNoopDriver())

				err = srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				// Publish messages
				for _, msg := range tc.messages {
					data, _ := json.Marshal(msg)
					err = qm.Publish(ctx, tc.topic, data)
					require.NoError(t, err, "Publishing message should succeed")
				}

				// Wait for message processing
				time.Sleep(2 * time.Second)

				mu.Lock()
				count := receivedCount
				mu.Unlock()
				require.Equal(t, tc.expectCount, count, "Should receive expected number of messages")

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceRegisterSubscriberWithError tests subscriber error handling.
func (s *QueueTestSuite) TestServiceRegisterSubscriberWithError() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
	}{
		{
			name:        "subscriber with error handler",
			serviceName: "Test Srv",
			topic:       "test-error",
			queueURL:    "mem://topicA",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error

				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				queueURL := tc.queueURL
				if queue != nil {
					queueSubject := strings.Replace(queueURL, "mem://", "", 1) + dep.Prefix()

					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						queueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", queueSubject).
							ExtendQuery("stream_name", queueSubject).
							ExtendQuery("stream_subjects", queueSubject).
							ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
							ExtendQuery("consumer_filter_subject", queueSubject).
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						queueURL = qDS.ExtendPath(queueSubject).String()
					}
				}

				handler := &handlerWithError{}
				opt := frame.WithRegisterSubscriber(tc.topic, queueURL, handler)
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), opt, frametests.WithNoopDriver())

				err = srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				subscriber, err := qm.GetSubscriber(tc.topic)
				require.NoError(t, err, "Could not get subscriber")
				require.NotNil(t, subscriber, "Subscriber should be registered")
				require.True(t, subscriber.Initiated(), "Subscriber should be initiated")

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceRegisterSubscriberInvalid tests invalid subscriber registration.
func (s *QueueTestSuite) TestServiceRegisterSubscriberInvalid() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
	}{
		{
			name:        "invalid subscriber registration",
			serviceName: "Test Srv",
			topic:       "test-invalid",
			queueURL:    "invalid://url",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				handler := &msgHandler{f: func(_ context.Context, _ map[string]string, _ []byte) error {
					return nil
				}}
				opt := frame.WithRegisterSubscriber(tc.topic, tc.queueURL, handler)
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), opt, frametests.WithNoopDriver())

				err := srv.Run(ctx, "")
				require.Error(t, err, "Service should fail to start with invalid queueManager URL")
			})
		}
	})
}

// TestServiceRegisterSubscriberContextCancelWorks tests context cancellation.
func (s *QueueTestSuite) TestServiceRegisterSubscriberContextCancelWorks() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
	}{
		{
			name:        "subscriber context cancellation",
			serviceName: "Test Srv",
			topic:       "test-cancel",
			queueURL:    "mem://topicA",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error

				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				queueURL := tc.queueURL
				if queue != nil {
					queueSubject := strings.Replace(queueURL, "mem://", "", 1) + dep.Prefix()

					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						queueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", queueSubject).
							ExtendQuery("stream_name", queueSubject).
							ExtendQuery("stream_subjects", queueSubject).
							ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
							ExtendQuery("consumer_filter_subject", queueSubject).
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						queueURL = qDS.ExtendPath(queueSubject).String()
					}
				}

				handler := &msgHandler{f: func(_ context.Context, _ map[string]string, _ []byte) error {
					return nil
				}}
				opt := frame.WithRegisterSubscriber(tc.topic, queueURL, handler)
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), opt, frametests.WithNoopDriver())

				err = srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				subscriber, err := qm.GetSubscriber(tc.topic)
				require.NoError(t, err, "Subscriber not found")
				require.NotNil(t, subscriber, "Subscriber should be registered")
				require.True(t, subscriber.Initiated(), "Subscriber should be initiated")

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceAddPublisher tests adding publishers dynamically.
func (s *QueueTestSuite) TestServiceAddPublisher() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
		message     []byte
	}{
		{
			name:        "add publisher dynamically",
			serviceName: "Test Srv",
			topic:       "test-add-publisher",
			queueURL:    "mem://topicA",
			message:     []byte("dynamic message"),
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error

				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				queueURL := tc.queueURL
				if queue != nil {
					queueSubject := strings.Replace(queueURL, "mem://", "", 1) + dep.Prefix()

					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						queueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", queueSubject).
							ExtendQuery("stream_name", queueSubject).
							ExtendQuery("stream_subjects", queueSubject).
							ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
							ExtendQuery("consumer_filter_subject", queueSubject).
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						queueURL = qDS.ExtendPath(queueSubject).String()
					}
				}

				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), frametests.WithNoopDriver())

				err = srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				err = qm.AddPublisher(ctx, tc.topic, queueURL)
				require.NoError(t, err, "Adding publisher should succeed")

				err = qm.Publish(ctx, tc.topic, tc.message)
				require.NoError(t, err, "Publishing to dynamically added topic should succeed")

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceAddPublisherInvalidURL tests adding publisher with invalid URL.
func (s *QueueTestSuite) TestServiceAddPublisherInvalidURL() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
	}{
		{
			name:        "add publisher with invalid URL",
			serviceName: "Test Srv",
			topic:       "test-invalid-url",
			queueURL:    "invalid://url",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), frametests.WithNoopDriver())

				err := srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				err = qm.AddPublisher(ctx, tc.topic, tc.queueURL)
				require.Error(t, err, "Adding publisher with invalid URL should fail")
			})
		}
	})
}

// TestServiceAddSubscriber tests adding subscribers dynamically.
func (s *QueueTestSuite) TestServiceAddSubscriber() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
	}{
		{
			name:        "add subscriber dynamically",
			serviceName: "Test Srv",
			topic:       "test-add-subscriber",
			queueURL:    "mem://topicA",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error

				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				queueURL := tc.queueURL
				if queue != nil {
					queueSubject := strings.Replace(queueURL, "mem://", "", 1) + dep.Prefix()

					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						queueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", queueSubject).
							ExtendQuery("stream_name", queueSubject).
							ExtendQuery("stream_subjects", queueSubject).
							ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
							ExtendQuery("consumer_filter_subject", queueSubject).
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						queueURL = qDS.ExtendPath(queueSubject).String()
					}
				}

				handler := &msgHandler{f: func(_ context.Context, _ map[string]string, _ []byte) error {
					return nil
				}}

				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), frametests.WithNoopDriver())

				err = srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				err = qm.AddSubscriber(ctx, tc.topic, queueURL, handler)
				require.NoError(t, err, "Adding subscriber should succeed")

				subscriber, err := qm.GetSubscriber(tc.topic)
				require.NoError(t, err)
				require.NotNil(t, subscriber, "Subscriber should be registered")
				require.True(t, subscriber.Initiated(), "Subscriber should be initiated")

				srv.Stop(ctx)
			})
		}
	})
}

// TestServiceAddSubscriberWithoutHandler tests adding subscriber without handler.
func (s *QueueTestSuite) TestServiceAddSubscriberWithoutHandler() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
	}{
		{
			name:        "add subscriber without handler",
			serviceName: "Test Srv",
			topic:       "test-no-handler",
			queueURL:    "mem://topicA",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var err error

				ctx := t.Context()
				queue := dep.ByIsQueue(ctx)

				queueURL := tc.queueURL
				if queue != nil {
					queueSubject := strings.Replace(queueURL, "mem://", "", 1) + dep.Prefix()

					qDS := queue.GetDS(ctx)
					if qDS.IsNats() {
						qDS, err = qDS.WithUser("ant")
						require.NoError(t, err)

						qDS, err = qDS.WithPassword("s3cr3t")
						require.NoError(t, err)

						queueURL = qDS.
							ExtendQuery("jetstream", "true").
							ExtendQuery("subject", queueSubject).
							ExtendQuery("stream_name", queueSubject).
							ExtendQuery("stream_subjects", queueSubject).
							ExtendQuery("consumer_durable_name", "Durable_"+queueSubject).
							ExtendQuery("consumer_filter_subject", queueSubject).
							ExtendQuery("consumer_ack_policy", "explicit").
							ExtendQuery("consumer_deliver_policy", "all").
							ExtendQuery("consumer_replay_policy", "instant").
							ExtendQuery("stream_retention", "workqueue").
							ExtendQuery("stream_storage", "file").
							String()
					} else {
						queueURL = qDS.ExtendPath(queueSubject).String()
					}
				}

				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), frametests.WithNoopDriver())

				err = srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				err = qm.AddSubscriber(ctx, tc.topic, queueURL)
				require.NoError(
					t,
					err,
					"Adding subscriber without handler should be ok as its effectively a pull subscriber now",
				)
			})
		}
	})
}

// TestServiceAddSubscriberInvalidURL tests adding subscriber with invalid URL.
func (s *QueueTestSuite) TestServiceAddSubscriberInvalidURL() {
	testCases := []struct {
		name        string
		serviceName string
		topic       string
		queueURL    string
	}{
		{
			name:        "add subscriber with invalid URL",
			serviceName: "Test Srv",
			topic:       "test-invalid-url",
			queueURL:    "invalid://url",
		},
	}

	s.WithTestDependancies(s.T(), func(t *testing.T, _ *definition.DependencyOption) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				handler := &msgHandler{f: func(_ context.Context, _ map[string]string, _ []byte) error {
					return nil
				}}

				ctx, srv := frame.NewService(frame.WithName(tc.serviceName), frametests.WithNoopDriver())

				err := srv.Run(ctx, "")
				require.NoError(t, err, "Service should start successfully")

				qm := srv.QueueManager()
				err = qm.AddSubscriber(ctx, tc.topic, tc.queueURL, handler)
				require.Error(t, err, "Adding subscriber with invalid URL should fail")
			})
		}
	})
}
