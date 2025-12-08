package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/frame/workerpool"
)

// testConfig implements ConfigurationWorkerPool for testing.
type testConfig struct{}

func (t *testConfig) GetCPUFactor() int                { return 1 }
func (t *testConfig) GetCapacity() int                 { return 10 }
func (t *testConfig) GetCount() int                    { return 1 }
func (t *testConfig) GetExpiryDuration() time.Duration { return time.Minute }

func TestAddSubscriberValidation(t *testing.T) {
	ctx := context.Background()
	cfg := &testConfig{}
	workPool := workerpool.NewManager(ctx, cfg, func(_ context.Context, _ error) {})
	qm := queue.NewQueueManager(ctx, workPool)

	tests := []struct {
		name        string
		reference   string
		queueURL    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty reference should fail",
			reference:   "",
			queueURL:    "nats://localhost:4222",
			expectError: true,
			errorMsg:    "subscriber reference cannot be empty",
		},
		{
			name:        "whitespace-only reference should fail",
			reference:   "   ",
			queueURL:    "nats://localhost:4222",
			expectError: true,
			errorMsg:    "subscriber reference cannot be empty",
		},
		{
			name:        "empty queueURL should fail",
			reference:   "test-subscriber",
			queueURL:    "",
			expectError: true,
			errorMsg:    "subscriber queueURL cannot be empty",
		},
		{
			name:        "whitespace-only queueURL should fail",
			reference:   "test-subscriber",
			queueURL:    "   ",
			expectError: true,
			errorMsg:    "subscriber queueURL cannot be empty",
		},
		{
			name:        "valid parameters should succeed",
			reference:   "test-subscriber",
			queueURL:    "nats://localhost:4222",
			expectError: false,
			errorMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := qm.AddSubscriber(ctx, tt.reference, tt.queueURL)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestAddPublisherValidation(t *testing.T) {
	ctx := context.Background()
	cfg := &testConfig{}
	workPool := workerpool.NewManager(ctx, cfg, func(_ context.Context, _ error) {})
	qm := queue.NewQueueManager(ctx, workPool)

	tests := []struct {
		name        string
		reference   string
		queueURL    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty reference should fail",
			reference:   "",
			queueURL:    "nats://localhost:4222",
			expectError: true,
			errorMsg:    "publisher reference cannot be empty",
		},
		{
			name:        "whitespace-only reference should fail",
			reference:   "   ",
			queueURL:    "nats://localhost:4222",
			expectError: true,
			errorMsg:    "publisher reference cannot be empty",
		},
		{
			name:        "empty queueURL should fail",
			reference:   "test-publisher",
			queueURL:    "",
			expectError: true,
			errorMsg:    "publisher queueURL cannot be empty",
		},
		{
			name:        "whitespace-only queueURL should fail",
			reference:   "test-publisher",
			queueURL:    "   ",
			expectError: true,
			errorMsg:    "publisher queueURL cannot be empty",
		},
		{
			name:        "valid parameters should succeed",
			reference:   "test-publisher",
			queueURL:    "nats://localhost:4222",
			expectError: false,
			errorMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := qm.AddPublisher(ctx, tt.reference, tt.queueURL)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}
