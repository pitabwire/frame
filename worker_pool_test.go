package frame_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pitabwire/frame"
)

type fields struct {
	test    string
	counter int
}

func (f *fields) process(_ context.Context, _ frame.JobResultPipe[any]) error {
	if f.test == "first error" {
		f.counter++
		f.test = "erred"
		return errors.New("test error")
	}

	f.test = "confirmed"
	return nil
}

// TestJobImpl_ChannelOperations tests the JobImpl's channel operations,
// specifically to ensure the WriteError and WriteResult methods correctly
// handle closed channels as fixed in the related bug.
func TestJobImpl_ChannelOperations(t *testing.T) {
	t.Run("WriteError and WriteResult should handle closed channels", func(t *testing.T) {
		ctx := t.Context()

		// Create a job
		job := frame.NewJob(func(_ context.Context, _ frame.JobResultPipe[any]) error {
			return nil
		})

		// First verify we can write to the channel
		err := job.WriteResult(ctx, "test result")
		if err != nil {
			t.Errorf("WriteResult to open channel failed: %v", err)
		}

		err = job.WriteError(ctx, errors.New("test error"))
		if err != nil {
			t.Errorf("WriteError to open channel failed: %v", err)
		}

		// Now close the channel
		job.Close()

		// Verify we get an error when trying to write to a closed channel
		err = job.WriteResult(ctx, "after close")
		if err == nil {
			t.Error("WriteResult should return an error when channel is closed")
		} else if !errors.Is(err, frame.ErrWorkerPoolResultChannelIsClosed) {
			t.Errorf("Expected ErrWorkerPoolResultChannelIsClosed but got: %v", err)
		}

		err = job.WriteError(ctx, errors.New("after close"))
		if err == nil {
			t.Error("WriteError should return an error when channel is closed")
		} else if !errors.Is(err, frame.ErrWorkerPoolResultChannelIsClosed) {
			t.Errorf("Expected ErrWorkerPoolResultChannelIsClosed but got: %v", err)
		}

		// Drain the channel first
		for res := range job.ResultChan() {
			// Just drain any existing messages
			t.Logf("res: %+v", res)
		}
	})
}

func writeIntRangeAsResult(ctx context.Context, t *testing.T, job frame.Job[any], count int) {
	for i := range count {
		if err := job.WriteResult(ctx, i); err != nil {
			t.Errorf("Failed to write result: %v", err)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	job.Close() // Close channel when done writing
}

// to ensure it properly handles multiple goroutines writing and reading.
func TestJobImpl_SafeConcurrentOperations(t *testing.T) {
	t.Run("Concurrent reads and writes should be safe", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		job := frame.NewJobWithBuffer(func(_ context.Context, _ frame.JobResultPipe[any]) error {
			return nil
		}, 10)

		// Writer goroutine
		go writeIntRangeAsResult(ctx, t, job, 5)

		// Reader goroutine
		count := 0
		for {
			result, ok := job.ReadResult(ctx)
			if !ok {
				// Channel closed or context canceled
				break
			}

			if result.IsError() {
				t.Errorf("Unexpected error: %v", result.Error())
			} else {
				count++
			}
		}

		if count == 0 {
			t.Error("Should have read at least one result")
		}
	})
}

func TestJobImpl_ChaoticConcurrentOperations(t *testing.T) {
	t.Run("Close should prevent further writes but allow reads", func(t *testing.T) {
		ctx := t.Context()
		job := frame.NewJobWithBuffer(func(_ context.Context, _ frame.JobResultPipe[any]) error {
			return nil
		}, 5)

		// Write some data
		for i := range 3 {
			if err := job.WriteResult(ctx, i); err != nil {
				t.Fatalf("Failed to write result: %v", err)
			}
		}

		// Close the channel
		job.Close()

		// Attempt to write should fail
		err := job.WriteResult(ctx, "should fail")
		if err == nil {
			t.Error("Write after close should fail")
		}

		// Should still be able to read existing data
		count := 0
		for {
			result, ok := job.ReadResult(ctx)
			if !ok {
				break
			}
			count++
			if result.IsError() {
				t.Errorf("Unexpected error result: %v", result.Error())
			}
		}

		if count != 3 {
			t.Errorf("Expected to read 3 items, got %d", count)
		}
	})
}

// correctly tracks the closed state of the channel.
func TestJobImpl_ResultChannelDoneFlag(t *testing.T) {
	t.Run("resultChanDone flag should properly indicate closed state", func(t *testing.T) {
		ctx := t.Context()
		job := frame.NewJob(func(_ context.Context, _ frame.JobResultPipe[any]) error {
			return nil
		})

		// Should be able to write before closing
		if err := job.WriteResult(ctx, "test"); err != nil {
			t.Errorf("Write before close failed: %v", err)
		}

		// Close multiple times should be safe
		job.Close()
		job.Close() // Should be idempotent

		// Write after closing should fail with the specific error
		if err := job.WriteError(ctx, errors.New("test")); !errors.Is(err, frame.ErrWorkerPoolResultChannelIsClosed) {
			t.Errorf("Expected ErrWorkerPoolResultChannelIsClosed but got: %v", err)
		}
	})
}

// TestJobImpl_SafeChannelOperations tests safe channel operations
// to ensure they properly handle context cancellation and closed channels.
func TestJobImpl_SafeChannelOperations(t *testing.T) {
	t.Run("Channel operations should respect context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		job := frame.NewJobWithBuffer(func(_ context.Context, _ frame.JobResultPipe[any]) error {
			return nil
		}, 1)

		// Write something to the channel first
		err := job.WriteResult(ctx, "test value")
		if err != nil {
			t.Fatalf("Failed to write to channel: %v", err)
		}

		// Now cancel the context
		cancel()

		// Attempt to write after context cancellation
		err = job.WriteResult(ctx, "should fail")
		if err == nil {
			t.Error("Writing after context cancellation should fail")
		}

		// Attempt to read after context cancellation
		_, ok := job.ReadResult(ctx)
		if ok {
			t.Error("Reading after context cancellation should return false")
		}
	})
}

func TestJobImpl_Process(t *testing.T) {
	tests := []struct {
		name    string
		fields  fields
		runs    int
		wantErr bool
	}{
		{
			name:    "Happy path",
			fields:  fields{},
			runs:    1,
			wantErr: false,
		}, {
			name: "Happy path 2",
			fields: fields{
				test: "overriden",
			},
			runs:    1,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, srv := frame.NewService(tt.name,
				frame.WithNoopDriver(),
				frame.WithBackgroundConsumer(func(_ context.Context) error {
					return nil
				}))

			err := srv.Run(ctx, ":")
			if err != nil {
				t.Errorf("could not start a background consumer peacefully : %v", err)
			}

			job := frame.NewJob(tt.fields.process)

			if err = frame.SubmitJob(ctx, srv, job); (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}

			time.Sleep(50 * time.Millisecond)

			if tt.runs != job.Runs() {
				t.Errorf("Test error could not retry for some reason, expected %d runs got %d ", tt.runs, job.Runs())
			}
		})
	}
}

func TestService_NewJobWithRetry(t *testing.T) {
	tests := []struct {
		name    string
		fields  fields
		runs    int
		wantErr bool
	}{
		{
			name: "Happy path",
			fields: fields{
				test: "first error",
			},
			runs:    2,
			wantErr: false,
		}, {
			name: "Happy path no error",
			fields: fields{
				test: "first error",
			},
			runs:    2,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, srv := frame.NewService(tt.name,
				frame.WithNoopDriver(),
				frame.WithBackgroundConsumer(func(_ context.Context) error {
					return nil
				}))

			err := srv.Run(ctx, ":")
			if err != nil {
				t.Errorf("could not start a background consumer peacefully : %v", err)
			}

			job := frame.NewJobWithRetry(tt.fields.process, 1)

			if err = frame.SubmitJob(ctx, srv, job); (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}

			time.Sleep(50 * time.Millisecond)

			if tt.runs != job.Runs() {
				t.Errorf("Test error could not retry for some reason")
			}
		})
	}
}

func TestService_NewJobWithBufferAndRetry(t *testing.T) {
	tests := []struct {
		name    string
		fields  fields
		runs    int
		wantErr bool
	}{
		{
			name: "Happy path",
			fields: fields{
				test: "first error",
			},
			runs:    2,
			wantErr: false,
		}, {
			name: "Happy path no error",
			fields: fields{
				test: "first error",
			},
			runs:    2,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, srv := frame.NewService(tt.name,
				frame.WithNoopDriver(),
				frame.WithBackgroundConsumer(func(_ context.Context) error {
					return nil
				}))

			err := srv.Run(ctx, ":")
			if err != nil {
				t.Errorf("could not start a background consumer peacefully : %v", err)
			}

			job := frame.NewJobWithBufferAndRetry(tt.fields.process, 4, 1)

			err = frame.SubmitJob(ctx, srv, job)
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}

			select {
			case _, ok := <-job.ResultChan():
				if !ok {
					t.Logf("result chan closed")
				}
			case <-time.Tick(500 * time.Millisecond):
				t.Fatalf("could not handle job within timelimit")
			}

			if tt.runs != job.Runs() {
				t.Errorf("Test retry count is not consistent: expected : %d got : %d ", tt.runs, job.Runs())
			}
		})
	}
}
