package frame_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pitabwire/frame"
)

func TestSubscriberMetrics_IsIdle(t *testing.T) {
	tests := []struct {
		name            string
		state           frame.SubscriberState
		activeMessages  int64
		expectedIsIdle  bool
	}{
		{
			name:           "Waiting state with zero active messages",
			state:          frame.SubscriberStateWaiting,
			activeMessages: 0,
			expectedIsIdle: true,
		},
		{
			name:           "Waiting state with active messages",
			state:          frame.SubscriberStateWaiting,
			activeMessages: 1,
			expectedIsIdle: false,
		},
		{
			name:           "Processing state with zero active messages",
			state:          frame.SubscriberStateProcessing,
			activeMessages: 0,
			expectedIsIdle: false,
		},
		{
			name:           "Error state with zero active messages",
			state:          frame.SubscriberStateInError,
			activeMessages: 0,
			expectedIsIdle: false,
		},
		{
			name:           "Edge case: Waiting state with negative active messages",
			state:          frame.SubscriberStateWaiting,
			activeMessages: -1,
			expectedIsIdle: false, // Current implementation checks for == 0, so this should fail
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metrics := &frame.SubscriberMetrics{
				ActiveMessages: &atomic.Int64{},
				LastActivity:   &atomic.Int64{},
				ProcessingTime: &atomic.Int64{},
				MessageCount:   &atomic.Int64{},
				ErrorCount:     &atomic.Int64{},
			}
			
			metrics.ActiveMessages.Store(tc.activeMessages)
			result := metrics.IsIdle(tc.state)
			
			if result != tc.expectedIsIdle {
				t.Errorf("IsIdle() = %v, expected %v", result, tc.expectedIsIdle)
			}
		})
	}
}

func TestSubscriberMetrics_IdleTime(t *testing.T) {
	metrics := &frame.SubscriberMetrics{
		ActiveMessages: &atomic.Int64{},
		LastActivity:   &atomic.Int64{},
		ProcessingTime: &atomic.Int64{},
		MessageCount:   &atomic.Int64{},
		ErrorCount:     &atomic.Int64{},
	}

	// Set last activity to 10 seconds ago
	metrics.LastActivity.Store(time.Now().Add(-10 * time.Second).UnixNano())

	// Test when idle
	idleTime := metrics.IdleTime(frame.SubscriberStateWaiting)
	if idleTime < 9*time.Second || idleTime > 11*time.Second {
		t.Errorf("IdleTime() = %v, expected approximately 10 seconds", idleTime)
	}

	// Test when not idle (processing state)
	idleTime = metrics.IdleTime(frame.SubscriberStateProcessing)
	if idleTime != 0 {
		t.Errorf("IdleTime() = %v, expected 0 for non-idle state", idleTime)
	}

	// Test when active messages > 0
	metrics.ActiveMessages.Store(1)
	idleTime = metrics.IdleTime(frame.SubscriberStateWaiting)
	if idleTime != 0 {
		t.Errorf("IdleTime() = %v, expected 0 when active messages > 0", idleTime)
	}
}

func TestSubscriberMetrics_AverageProcessingTime(t *testing.T) {
	metrics := &frame.SubscriberMetrics{
		ActiveMessages: &atomic.Int64{},
		LastActivity:   &atomic.Int64{},
		ProcessingTime: &atomic.Int64{},
		MessageCount:   &atomic.Int64{},
		ErrorCount:     &atomic.Int64{},
	}

	// Initial state should return zero
	avgTime := metrics.AverageProcessingTime()
	if avgTime != 0 {
		t.Errorf("Initial AverageProcessingTime() = %v, expected 0", avgTime)
	}

	// Set some processing time and message count
	metrics.ProcessingTime.Store(500 * int64(time.Millisecond))
	metrics.MessageCount.Store(5)

	// Test average calculation
	avgTime = metrics.AverageProcessingTime()
	expected := time.Duration(100 * time.Millisecond)
	if avgTime != expected {
		t.Errorf("AverageProcessingTime() = %v, expected %v", avgTime, expected)
	}
}

func TestSubscriberMetrics_ConcurrentAccess(t *testing.T) {
	metrics := &frame.SubscriberMetrics{
		ActiveMessages: &atomic.Int64{},
		LastActivity:   &atomic.Int64{},
		ProcessingTime: &atomic.Int64{},
		MessageCount:   &atomic.Int64{},
		ErrorCount:     &atomic.Int64{},
	}
	
	// Test concurrent access to atomic counters
	const numGoroutines = 100
	const numOperationsPerGoroutine = 100
	
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			
			// Simulate message processing operations
			for j := 0; j < numOperationsPerGoroutine; j++ {
				// Increment active messages
				metrics.ActiveMessages.Add(1)
				
				// Update last activity
				metrics.LastActivity.Store(time.Now().UnixNano())
				
				// Add some processing time
				metrics.ProcessingTime.Add(int64(time.Millisecond * 5))
				
				// Increment message count
				metrics.MessageCount.Add(1)
				
				// Decrement active messages
				metrics.ActiveMessages.Add(-1)
				
				// Sometimes add an error
				if j%10 == 0 {
					metrics.ErrorCount.Add(1)
				}
			}
		}()
	}
	
	wg.Wait()
	
	// Verify results
	expectedMessages := int64(numGoroutines * numOperationsPerGoroutine)
	expectedErrors := int64(numGoroutines * numOperationsPerGoroutine / 10)
	
	if msgs := metrics.MessageCount.Load(); msgs != expectedMessages {
		t.Errorf("MessageCount = %d, expected %d", msgs, expectedMessages)
	}
	
	if errs := metrics.ErrorCount.Load(); errs != expectedErrors {
		t.Errorf("ErrorCount = %d, expected %d", errs, expectedErrors)
	}
	
	// Active messages should be back to zero after all operations completed
	if active := metrics.ActiveMessages.Load(); active != 0 {
		t.Errorf("ActiveMessages = %d, expected 0 after all operations complete", active)
	}
}

func TestSubscriberMetrics_IntegrationWithSubscriber(t *testing.T) {
	// Create a memory-based queue for testing
	regSubTopic := "test-metrics-subscriber"
	queueURL := "mem://metrics-test-topic"

	var successfulMessages int64
	var messageProcessingTime int64
	
	// Create a handler that tracks metrics
	handler := &msgHandler{f: func(ctx context.Context, metadata map[string]string, message []byte) error {
		// Simulate some work
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&successfulMessages, 1)
		atomic.AddInt64(&messageProcessingTime, int64(10 * time.Millisecond))
		return nil
	}}

	// Set up service with publisher and subscriber
	optTopic := frame.WithRegisterPublisher(regSubTopic, queueURL)
	opt := frame.WithRegisterSubscriber(regSubTopic, queueURL, handler)
	ctx, srv := frame.NewService("Metrics Test Srv", optTopic, opt, frame.WithNoopDriver())
	
	defer srv.Stop(ctx)

	// Run the service
	err := srv.Run(ctx, "")
	if err != nil {
		t.Fatalf("Failed to run service: %v", err)
	}
	
	// Get the subscriber to access metrics
	sub, err := srv.GetSubscriber(regSubTopic)
	if err != nil {
		t.Fatalf("Could not get subscriber: %v", err)
	}
	
	// Verify initial metrics state
	metrics := sub.Metrics()
	if metrics == nil {
		t.Fatal("Expected subscriber to have metrics")
	}
	
	// Initial state should be idle with no messages processed
	initialIsIdle := sub.IsIdle()
	initialMsgCount := metrics.MessageCount.Load()
	
	// Publish messages
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		err = srv.Publish(ctx, regSubTopic, []byte("metrics test message"))
		if err != nil {
			t.Errorf("Failed to publish message: %v", err)
		}
	}
	
	// Wait for messages to be processed
	time.Sleep(500 * time.Millisecond)
	
	// Verify metrics were updated
	finalMsgCount := metrics.MessageCount.Load()
	finalAvgTime := metrics.AverageProcessingTime()
	
	// Message count should have increased
	if finalMsgCount <= initialMsgCount {
		t.Errorf("Message count did not increase: initial=%d, final=%d", initialMsgCount, finalMsgCount)
	}
	
	// Average processing time should be reasonable
	if finalAvgTime < time.Millisecond || finalAvgTime > time.Second {
		t.Errorf("Unexpected average processing time: %v", finalAvgTime)
	}
	
	// After processing messages, should eventually return to idle
	// Wait a bit to allow for the subscriber to become idle again
	time.Sleep(100 * time.Millisecond)
	
	// Check if subscriber has returned to idle state
	if !sub.IsIdle() && initialIsIdle {
		t.Error("Expected subscriber to return to idle state after processing")
	}
}
