package frame

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/rs/xid"
)

const defaultJobResultBufferSize = 10

type JobResult[J any] interface {
	IsError() bool
	Error() error
	Item() J
}

type jobResult[J any] struct {
	item  J
	error error
}

func (j *jobResult[J]) IsError() bool {
	return j.error != nil
}

func (j *jobResult[J]) Error() error {
	return j.error
}

func (j *jobResult[J]) Item() J {
	return j.item
}

type JobResultPipe[J any] interface {
	ResultBufferSize() int
	ResultChan() <-chan JobResult[J]
	WriteError(ctx context.Context, val error) error
	WriteResult(ctx context.Context, val J) error
	ReadResult(ctx context.Context) (JobResult[J], bool)
	IsClosed() bool
	Close()
}

type Job[J any] interface {
	JobResultPipe[J]
	F() func(ctx context.Context, result JobResultPipe[J]) error
	ID() string
	CanRun() bool
	Retries() int
	Runs() int
	IncreaseRuns()
}

type JobImpl[J any] struct {
	id               string
	runs             int
	retries          int
	resultBufferSize int
	resultChan       chan JobResult[J]
	resultMu         sync.Mutex
	resultChanDone   bool
	processFunc      func(ctx context.Context, result JobResultPipe[J]) error
}

func (ji *JobImpl[J]) ID() string {
	return ji.id
}

func (ji *JobImpl[J]) F() func(ctx context.Context, result JobResultPipe[J]) error {
	return ji.processFunc
}

func (ji *JobImpl[J]) CanRun() bool {
	return ji.Retries() > (ji.Runs() - 1)
}

func (ji *JobImpl[J]) Retries() int {
	return ji.retries
}

func (ji *JobImpl[J]) Runs() int {
	return ji.runs
}

func (ji *JobImpl[J]) IncreaseRuns() {
	ji.resultMu.Lock()
	defer ji.resultMu.Unlock()

	ji.runs++
}

func (ji *JobImpl[J]) ResultBufferSize() int {
	return ji.resultBufferSize
}

func (ji *JobImpl[J]) ResultChan() <-chan JobResult[J] {
	return ji.resultChan
}

func (ji *JobImpl[J]) ReadResult(ctx context.Context) (JobResult[J], bool) {
	return SafeChannelRead(ctx, ji.resultChan)
}

func (ji *JobImpl[J]) WriteError(ctx context.Context, val error) error {
	return SafeChannelWrite(ctx, ji.resultChan, &jobResult[J]{error: val})
}

func (ji *JobImpl[J]) WriteResult(ctx context.Context, val J) error {
	return SafeChannelWrite(ctx, ji.resultChan, &jobResult[J]{item: val})
}

func (ji *JobImpl[J]) Close() {
	ji.resultMu.Lock()
	defer ji.resultMu.Unlock()
	if !ji.resultChanDone {
		close(ji.resultChan)
		ji.resultChanDone = true
	}
}

func (ji *JobImpl[J]) IsClosed() bool {
	ji.resultMu.Lock()
	defer ji.resultMu.Unlock()
	return ji.resultChanDone
}

func NewJob[J any](process func(ctx context.Context, result JobResultPipe[J]) error) Job[J] {
	return NewJobWithBufferAndRetry[J](process, defaultJobResultBufferSize, 0)
}

func NewJobWithBuffer[J any](process func(ctx context.Context, result JobResultPipe[J]) error, buffer int) Job[J] {
	return NewJobWithBufferAndRetry(process, buffer, 0)
}

func NewJobWithRetry[J any](process func(ctx context.Context, result JobResultPipe[J]) error, retries int) Job[J] {
	return NewJobWithBufferAndRetry(process, defaultJobResultBufferSize, retries)
}

func NewJobWithBufferAndRetry[J any](
	process func(ctx context.Context, result JobResultPipe[J]) error,
	resultBufferSize, retries int,
) Job[J] {
	return &JobImpl[J]{
		id:               xid.New().String(),
		retries:          retries,
		processFunc:      process,
		resultBufferSize: resultBufferSize,
		resultChan:       make(chan JobResult[J], resultBufferSize),
	}
}

// WithBackgroundConsumer sets a background consumer function for the worker pool.
func WithBackgroundConsumer(deque func(_ context.Context) error) Option {
	return func(_ context.Context, s *Service) {
		s.backGroundClient = deque
	}
}

// WithPoolConcurrency sets the number of worker pool concurrency.
func WithPoolConcurrency(workers int) Option {
	return func(_ context.Context, s *Service) {
		s.poolWorkerCount = workers
	}
}

// WithPoolCapacity sets the capacity of the worker pool.
func WithPoolCapacity(capacity int) Option {
	return func(_ context.Context, s *Service) {
		s.poolCapacity = capacity
	}
}

// createJobExecutionTask creates a new task function that encapsulates job execution, error handling, and retry logic.
func createJobExecutionTask[J any](ctx context.Context, s *Service, job Job[J]) func() {
	return func() {
		defer job.Close()

		if job.F() == nil {
			s.Log(ctx).WithField("job_id", job.ID()).Error("Job function (job.F()) is nil")
			_ = job.WriteError(ctx, errors.New("job function (job.F()) is nil"))
			return
		}

		job.IncreaseRuns()
		executionErr := job.F()(ctx, job)

		// Handle successful execution first and return early
		if executionErr == nil {
			s.Log(ctx).WithField("job_id", job.ID()).Debug("Job executed successfully")
			_ = job.WriteError(ctx, nil) // Report success (nil error)
			return
		}

		// At this point, executionErr != nil, so handle the error case.
		logger := s.Log(ctx).WithError(executionErr).
			WithField("job", job.ID()).
			WithField("retry", job.Retries())

		if job.CanRun() { // Check if job can be retried
			logger.Info("Job failed, attempting retry")
			resubmitErr := SubmitJob(ctx, s, job) // Recursive call to SubmitJob for retry
			if resubmitErr != nil {
				logger.WithError(resubmitErr).
					WithField("stacktrace", string(debug.Stack())).
					Error("Failed to resubmit job for retry. Reporting original execution error.")
				// If resubmission fails, the original error of this attempt should be reported.
				_ = job.WriteError(ctx, executionErr)
			} else {
				logger.Debug("Job successfully resubmitted for retry")
				// If successfully resubmitted, do not write current executionErr to job's channel;
				// the next attempt will handle its own outcome.
			}
		} else {
			// Job failed and cannot be retried (e.g., retries exhausted).
			logger.Error("Job failed; retries exhausted or job cannot run further. Reporting final error.")
			_ = job.WriteError(ctx, executionErr)
		}
	}
}

// SubmitJob used to submit jobs to our worker pool for processing.
// Once a job is submitted the end user does not need to do any further tasks
// One can ideally also wait for the results of their processing for their specific job
// This is done by simply by listening to the jobs ErrChan. Be sure to also check for when its closed
//
//	err, ok := <- errChan
func SubmitJob[J any](ctx context.Context, s *Service, job Job[J]) error {
	p := s.pool
	if p.IsClosed() {
		return errors.New("pool is closed")
	}

	// This select block makes the submission attempt itself cancellable by ctx.
	// It attempts to submit once.
	select {
	case <-ctx.Done():
		// Use ctx.Err() to provide a more specific error about context cancellation.
		return fmt.Errorf("context cancelled before job submission: %w", ctx.Err())
	default:
		// If job cannot run initially (e.g., retries already exhausted or other conditions),
		// the original code returned nil, implying not to attempt submission.
		if !job.CanRun() {
			s.Log(ctx).WithField("job_id", job.ID()).Info("Job cannot run (initial check), not submitting.")
			return nil
		}

		// Create the actual task to be executed by a worker.
		task := createJobExecutionTask(ctx, s, job)
		return p.Submit(task)
	}
}

// SafeChannelWrite writes a value to a channel, returning an error if the context is canceled.
func SafeChannelWrite[J any](ctx context.Context, ch chan<- JobResult[J], value JobResult[J]) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- value:
		return nil
	}
}

func SafeChannelRead[J any](ctx context.Context, ch <-chan JobResult[J]) (JobResult[J], bool) {
	select {
	case <-ctx.Done():
		// Return context error without blocking
		return &jobResult[J]{error: ctx.Err()}, false

	case result, ok := <-ch:
		// Channel read successfully or channel closed
		return result, ok
	}
}
