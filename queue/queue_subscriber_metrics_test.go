package queue_test

//
// import (
// 	"context"
// 	"sync"
// 	"sync/atomic"
// 	"testing"
// 	"time"
//
// 	"github.com/pitabwire/frame"
// 	"github.com/pitabwire/frame/frametests/definition"
// 	"github.com/pitabwire/frame/queue"
// 	"github.com/pitabwire/frame/tests"
// 	"github.com/stretchr/testify/require"
// 	"github.com/stretchr/testify/suite"
// )
//
// // QueueSubscriberMetricsTestSuite extends BaseTestSuite for comprehensive subscriber metrics testing.
// type QueueSubscriberMetricsTestSuite struct {
// 	tests.BaseTestSuite
// }
//
// // TestQueueSubscriberMetricsSuite runs the queue subscriber metrics test suite.
// func TestQueueSubscriberMetricsSuite(t *testing.T) {
// 	suite.Run(t, &QueueSubscriberMetricsTestSuite{})
// }
//
// // TestSubscriberMetricsIsIdle tests idle state detection.
// func (s *QueueSubscriberMetricsTestSuite) TestSubscriberMetricsIsIdle() {
// 	testCases := []struct {
// 		name           string
// 		state          queue.SubscriberState
// 		activeMessages int64
// 		expectedIsIdle bool
// 	}{
// 		{
// 			name:           "Waiting state with zero active messages",
// 			state:          queue.SubscriberStateWaiting,
// 			activeMessages: 0,
// 			expectedIsIdle: true,
// 		},
// 		{
// 			name:           "Waiting state with active messages",
// 			state:          queue.SubscriberStateWaiting,
// 			activeMessages: 1,
// 			expectedIsIdle: false,
// 		},
// 		{
// 			name:           "Processing state with zero active messages",
// 			state:          queue.SubscriberStateProcessing,
// 			activeMessages: 0,
// 			expectedIsIdle: false,
// 		},
// 		{
// 			name:           "Error state with zero active messages",
// 			state:          queue.SubscriberStateInError,
// 			activeMessages: 0,
// 			expectedIsIdle: false,
// 		},
// 		{
// 			name:           "Edge case: Waiting state with negative active messages",
// 			state:          queue.SubscriberStateWaiting,
// 			activeMessages: -1,
// 			expectedIsIdle: true, // Negative active messages should be treated as zero
// 		},
// 	}
//
// 	for _, tc := range testCases {
// 		s.Run(tc.name, func() {
// 			metrics := &queue.SubscriberMetrics{
// 				ActiveMessages: &atomic.Int64{},
// 				LastActivity:   &atomic.Int64{},
// 				ProcessingTime: &atomic.Int64{},
// 				MessageCount:   &atomic.Int64{},
// 				ErrorCount:     &atomic.Int64{},
// 			}
//
// 			metrics.ActiveMessages.Store(tc.activeMessages)
// 			result := metrics.IsIdle(tc.state)
// 			s.Require().Equal(tc.expectedIsIdle, result, "IsIdle() result should match expected")
// 		})
// 	}
// }
//
// // TestSubscriberMetricsIdleTime tests idle time calculation.
// func (s *QueueSubscriberMetricsTestSuite) TestSubscriberMetricsIdleTime() {
// 	testCases := []struct {
// 		name             string
// 		lastActivityAgo  time.Duration
// 		state            queue.SubscriberState
// 		activeMessages   int64
// 		expectedIdleTime time.Duration
// 	}{
// 		{
// 			name:             "idle time when waiting with no active messages",
// 			lastActivityAgo:  10 * time.Second,
// 			state:            queue.SubscriberStateWaiting,
// 			activeMessages:   0,
// 			expectedIdleTime: 10 * time.Second,
// 		},
// 		{
// 			name:             "idle time when processing",
// 			lastActivityAgo:  5 * time.Second,
// 			state:            queue.SubscriberStateProcessing,
// 			activeMessages:   0,
// 			expectedIdleTime: 0,
// 		},
// 		{
// 			name:             "idle time when waiting with active messages",
// 			lastActivityAgo:  3 * time.Second,
// 			state:            queue.SubscriberStateWaiting,
// 			activeMessages:   1,
// 			expectedIdleTime: 0,
// 		},
// 	}
//
// 	for _, tc := range testCases {
// 		s.Run(tc.name, func() {
// 			metrics := &queue.SubscriberMetrics{
// 				ActiveMessages: &atomic.Int64{},
// 				LastActivity:   &atomic.Int64{},
// 				ProcessingTime: &atomic.Int64{},
// 				MessageCount:   &atomic.Int64{},
// 				ErrorCount:     &atomic.Int64{},
// 			}
//
// 			// Set last activity to specified time ago
// 			metrics.LastActivity.Store(time.Now().Add(-tc.lastActivityAgo).UnixNano())
// 			metrics.ActiveMessages.Store(tc.activeMessages)
//
// 			idleTime := metrics.IdleTime(tc.state)
//
// 			if tc.expectedIdleTime == 0 {
// 				s.Require().Equal(time.Duration(0), idleTime, "Idle time should be 0")
// 			} else {
// 				// Allow some tolerance for timing
// 				s.Require().True(idleTime >= tc.expectedIdleTime-500*time.Millisecond &&
// 					idleTime <= tc.expectedIdleTime+500*time.Millisecond,
// 					"Idle time should be approximately %v, got %v", tc.expectedIdleTime, idleTime)
// 			}
// 		})
// 	}
// }
//
// // TestSubscriberMetricsAverageProcessingTime tests average processing time calculation.
// func (s *QueueSubscriberMetricsTestSuite) TestSubscriberMetricsAverageProcessingTime() {
// 	testCases := []struct {
// 		name                string
// 		totalProcessingTime time.Duration
// 		messageCount        int64
// 		expectedAverage     time.Duration
// 	}{
// 		{
// 			name:                "average processing time with messages",
// 			totalProcessingTime: 10 * time.Second,
// 			messageCount:        5,
// 			expectedAverage:     2 * time.Second,
// 		},
// 		{
// 			name:                "average processing time with zero messages",
// 			totalProcessingTime: 5 * time.Second,
// 			messageCount:        0,
// 			expectedAverage:     0,
// 		},
// 		{
// 			name:                "average processing time with single message",
// 			totalProcessingTime: 3 * time.Second,
// 			messageCount:        1,
// 			expectedAverage:     3 * time.Second,
// 		},
// 	}
//
// 	for _, tc := range testCases {
// 		s.Run(tc.name, func() {
// 			metrics := &queue.SubscriberMetrics{
// 				ActiveMessages: &atomic.Int64{},
// 				LastActivity:   &atomic.Int64{},
// 				ProcessingTime: &atomic.Int64{},
// 				MessageCount:   &atomic.Int64{},
// 				ErrorCount:     &atomic.Int64{},
// 			}
//
// 			metrics.ProcessingTime.Store(tc.totalProcessingTime.Nanoseconds())
// 			metrics.MessageCount.Store(tc.messageCount)
//
// 			average := metrics.AverageProcessingTime()
// 			s.Require().Equal(tc.expectedAverage, average, "Average processing time should match expected")
// 		})
// 	}
// }
//
// // TestSubscriberMetricsConcurrentAccess tests concurrent access to metrics.
// func (s *QueueSubscriberMetricsTestSuite) TestSubscriberMetricsConcurrentAccess() {
// 	testCases := []struct {
// 		name       string
// 		goroutines int
// 		operations int
// 	}{
// 		{
// 			name:       "concurrent access with multiple goroutines",
// 			goroutines: 10,
// 			operations: 100,
// 		},
// 	}
//
// 	for _, tc := range testCases {
// 		s.Run(tc.name, func() {
// 			metrics := &queue.SubscriberMetrics{
// 				ActiveMessages: &atomic.Int64{},
// 				LastActivity:   &atomic.Int64{},
// 				ProcessingTime: &atomic.Int64{},
// 				MessageCount:   &atomic.Int64{},
// 				ErrorCount:     &atomic.Int64{},
// 			}
//
// 			var wg sync.WaitGroup
// 			wg.Add(tc.goroutines)
//
// 			// Start multiple goroutines performing operations
// 			for range tc.goroutines {
// 				go func() {
// 					defer wg.Done()
// 					for range tc.operations {
// 						metrics.ActiveMessages.Add(1)
// 						metrics.MessageCount.Add(1)
// 						metrics.ProcessingTime.Add(1000000) // 1ms in nanoseconds
// 						metrics.LastActivity.Store(time.Now().UnixNano())
// 						metrics.ErrorCount.Add(0)
//
// 						// Read operations
// 						_ = metrics.IsIdle(frame.SubscriberStateWaiting)
// 						_ = metrics.IdleTime(frame.SubscriberStateWaiting)
// 						_ = metrics.AverageProcessingTime()
// 					}
// 				}()
// 			}
//
// 			wg.Wait()
//
// 			// Verify final state
// 			expectedTotal := int64(tc.goroutines * tc.operations)
// 			s.Require().Equal(expectedTotal, metrics.MessageCount.Load(), "Message count should match expected total")
// 			s.Require().Equal(
// 				expectedTotal,
// 				metrics.ActiveMessages.Load(),
// 				"Active messages should match expected total",
// 			)
//
// 			expectedProcessingTime := expectedTotal * 1000000 // 1ms per operation
// 			s.Require().Equal(
// 				expectedProcessingTime,
// 				metrics.ProcessingTime.Load(),
// 				"Processing time should match expected total",
// 			)
// 		})
// 	}
// }
//
// TestSubscriberMetricsIntegrationWithSubscriber tests integration with subscriber.
// func (s *QueueSubscriberMetricsTestSuite) TestSubscriberMetricsIntegrationWithSubscriber() {
// 	testCases := []struct {
// 		name        string
// 		serviceName string
// 		topic       string
// 		queueURL    string
// 		messages    []string
// 	}{
// 		{
// 			name:        "integration with subscriber metrics",
// 			serviceName: "Test Subscriber Metrics",
// 			topic:       "test-metrics-integration",
// 			queueURL:    "mem://topicA",
// 			messages:    []string{"msg1", "msg2", "msg3"},
// 		},
// 	}
//
// 	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
// 		for _, tc := range testCases {
// 			t.Run(tc.name, func(t *testing.T) {
// 				queue := dep.ByIsQueue(t.Context())
//
// 				var processedCount int64
// 				var mu sync.Mutex
//
// 				handler := func(ctx context.Context, metadata map[string]string, message []byte) error {
// 					mu.Lock()
// 					processedCount++
// 					mu.Unlock()
// 					time.Sleep(10 * time.Millisecond) // Simulate processing time
// 					return nil
// 				}
//
// 				// Create subscriber with handler
// 				subscriber := queue.NewSubscriber(tc.topic, queue.GetDS(ctx).String(), handler)
//
// 				// Start subscriber
// 				err := subscriber.Start(ctx)
// 				require.NoError(t, err, "Subscriber should start successfully")
//
// 				// Publish messages
// 				for _, msg := range tc.messages {
// 					err = subscriber.Publish(ctx, []byte(msg))
// 					require.NoError(t, err, "Publishing message should succeed")
// 				}
//
// 				// Wait for processing
// 				time.Sleep(1 * time.Second)
//
// 				// Check metrics
// 				metrics := subscriber.Metrics()
// 				require.NotNil(t, metrics, "Metrics should not be nil")
//
// 				messageCount := metrics.MessageCount.Load()
// 				require.GreaterOrEqual(t, messageCount, int64(len(tc.messages)), "Message count should be at least the number of sent messages")
//
// 				// Verify processing time is reasonable
// 				processingTime := metrics.ProcessingTime.Load()
// 				require.Greater(t, processingTime, int64(0), "Processing time should be greater than 0")
//
// 				// Verify average processing time
// 				averageTime := metrics.AverageProcessingTime()
// 				require.Greater(t, averageTime, time.Duration(0), "Average processing time should be greater than 0")
//
// 				// Stop subscriber
// 				subscriber.Stop()
// 			})
// 		}
// 	})
// }
