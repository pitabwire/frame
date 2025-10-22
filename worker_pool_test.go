package frame_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/tests"
	"github.com/stretchr/testify/suite"
)

// WorkerPoolTestSuite extends BaseTestSuite for comprehensive worker pool testing.
type WorkerPoolTestSuite struct {
	tests.BaseTestSuite
}

// TestWorkerPoolSuite runs the worker pool test suite.
func TestWorkerPoolSuite(t *testing.T) {
	suite.Run(t, &WorkerPoolTestSuite{})
}

// TestJobImplChannelOperations tests the JobImpl's channel operations.
func (s *WorkerPoolTestSuite) TestJobImplChannelOperations() {
	testCases := []struct {
		name string
	}{
		{
			name: "WriteError and WriteResult should handle closed channels",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx := s.T().Context()

			// Create a job
			job := frame.NewJob(func(_ context.Context, _ frame.JobResultPipe[any]) error {
				return nil
			})

			// First verify we can write to the channel
			err := job.WriteResult(ctx, "test result")
			s.Require().NoError(err, "WriteResult to open channel should succeed")

			err = job.WriteError(ctx, errors.New("test error"))
			s.Require().NoError(err, "WriteError to open channel should succeed")

			// Now close the channel
			job.Close()

			// Verify we get an error when trying to write to a closed channel
			err = job.WriteResult(ctx, "after close")
			s.Require().Error(err, "WriteResult should return an error when channel is closed")
			s.Require().ErrorIs(err, frame.ErrWorkerPoolResultChannelIsClosed,
				"WriteResult should return ErrWorkerPoolResultChannelIsClosed")

			err = job.WriteError(ctx, errors.New("after close"))
			s.Require().Error(err, "WriteError should return an error when channel is closed")
			s.Require().ErrorIs(err, frame.ErrWorkerPoolResultChannelIsClosed,
				"WriteError should return ErrWorkerPoolResultChannelIsClosed")

			// Drain the channel first
			for res := range job.ResultChan() {
				// Just drain any existing messages
				s.T().Logf("res: %+v", res)
			}
		})
	}
}

func (s *WorkerPoolTestSuite) writeIntRangeAsResult(ctx context.Context, t *testing.T, job frame.Job[any], count int) {
	for i := range count {
		if err := job.WriteResult(ctx, i); err != nil {
			t.Errorf("Failed to write result: %v", err)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	job.Close() // Close channel when done writing
}

// TestJobImplSafeConcurrentOperations tests safe concurrent operations.
func (s *WorkerPoolTestSuite) TestJobImplSafeConcurrentOperations() {
	testCases := []struct {
		name       string
		timeout    time.Duration
		buffer     int
		writeCount int
	}{
		{
			name:       "Concurrent reads and writes should be safe",
			timeout:    500 * time.Millisecond,
			buffer:     10,
			writeCount: 5,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx, cancel := context.WithTimeout(s.T().Context(), tc.timeout)
			defer cancel()

			job := frame.NewJobWithBuffer(func(_ context.Context, _ frame.JobResultPipe[any]) error {
				return nil
			}, tc.buffer)

			// Writer goroutine
			go s.writeIntRangeAsResult(ctx, s.T(), job, tc.writeCount)

			// Reader goroutine
			go func() {
				for res := range job.ResultChan() {
					s.T().Logf("Received: %+v", res)
				}
			}()

			// Wait for completion or timeout
			<-ctx.Done()
			s.Require().True(errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled),
				"Context should be done by timeout or cancellation")
		})
	}
}

// TestJobImplChaoticConcurrentOperations tests chaotic concurrent operations.
//
//nolint:gocognit
func (s *WorkerPoolTestSuite) TestJobImplChaoticConcurrentOperations() {
	testCases := []struct {
		name       string
		goroutines int
		iterations int
		buffer     int
	}{
		{
			name:       "chaotic concurrent operations",
			goroutines: 5,
			iterations: 10,
			buffer:     20,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx, cancel := context.WithTimeout(s.T().Context(), 2*time.Second)
			defer cancel()

			job := frame.NewJobWithBuffer(func(_ context.Context, _ frame.JobResultPipe[any]) error {
				return nil
			}, tc.buffer)

			var wg sync.WaitGroup
			wg.Add(tc.goroutines)

			// Start multiple goroutines that write and read
			for i := range tc.goroutines {
				go func(id int) {
					defer wg.Done()
					for j := range tc.iterations {
						select {
						case <-ctx.Done():
							return
						default:
							err := job.WriteResult(ctx, id*tc.iterations+j)
							if err != nil {
								s.T().Errorf("Goroutine %d failed to write: %v", id, err)
								return
							}
							time.Sleep(time.Millisecond)
						}
					}
				}(i)
			}

			// Reader goroutine
			go func() {
				count := 0
				for range job.ResultChan() {
					count++
					if count >= tc.goroutines*tc.iterations {
						cancel()
						return
					}
				}
			}()

			wg.Wait()
			job.Close()
		})
	}
}

// TestJobImplSafeChannelOperations tests safe channel operations.
func (s *WorkerPoolTestSuite) TestJobImplSafeChannelOperations() {
	testCases := []struct {
		name    string
		timeout time.Duration
		buffer  int
	}{
		{
			name:    "safe channel operations should handle context cancellation",
			timeout: 100 * time.Millisecond,
			buffer:  5,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx, cancel := context.WithTimeout(s.T().Context(), tc.timeout)
			defer cancel()

			job := frame.NewJobWithBuffer(func(_ context.Context, _ frame.JobResultPipe[any]) error {
				return nil
			}, tc.buffer)

			// Try to write after context is cancelled
			go func() {
				time.Sleep(tc.timeout / 2)
				cancel()
			}()

			err := job.WriteResult(ctx, "test")
			if err != nil {
				s.Require().True(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
					"WriteResult should handle context cancellation gracefully")
			}

			job.Close()
		})
	}
}
