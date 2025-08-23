package frameworker

import (
	"context"
	"errors"
	"fmt"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/internal/common"
)

// All duplicate declarations moved to interface.go and manager.go to avoid duplication
// ErrWorkerPoolResultChannelIsClosed, WorkerPoolOptions, WorkerPoolOption, and all With* functions moved

// WithWorkerPoolOptions provides a way to set custom options for the ants worker pool.
// Renamed from WithAntsOptions and changed parameter type.
// WithWorkerPoolOptions moved to manager.go to avoid undefined types

// WorkerPool moved to interface.go to avoid duplication

// defaultWorkerPoolOpts moved to manager.go to avoid duplication

// setupWorkerPool moved to manager.go to avoid duplication

// singlePoolWrapper moved to manager.go to avoid duplication

// multiPoolWrapper moved to manager.go to avoid duplication

// defaultJobResultBufferSize and defaultJobRetryCount moved to manager.go to avoid duplication

// JobResult moved to interface.go to avoid duplication

// jobResult moved to job.go to avoid duplication

// Result and ErrorResult moved to job.go to avoid duplication

// JobResultPipe moved to interface.go to avoid duplication

// Job moved to interface.go to avoid duplication

// JobImpl moved to job.go to avoid duplication

// NewJob* functions moved to job.go to avoid duplication

// WithBackgroundConsumer sets a background consumer function for the worker pool.
func WithBackgroundConsumer(deque func(_ context.Context) error) common.Option {
	return func(_ context.Context, s common.Service) {
		if workerPoolModule, exists := s.GetModule(common.ModuleTypeWorkerPool).(common.WorkerPoolModule); exists {
			// Update WorkerPoolModule with background client
			newModule := common.NewWorkerPoolModule(
				workerPoolModule.Pool(),
				workerPoolModule.PoolOptions(),
				deque, // Set background client
			)
			s.RegisterModule(newModule)
		}
	}
}

// SubmitJob used to submit jobs to our worker pool for processing.
// Once a job is submitted the end user does not need to do any further tasks
// One can ideally also wait for the results of their processing for their specific job
// by listening to the job's ResultChan.
func SubmitJob[T any](ctx context.Context, s *common.Service, job Job[T]) error {
	if s == nil {
		return errors.New("service is nil")
	}

	// Get pool from WorkerPoolModule
	workerPoolModule, exists := (*s).GetModule(common.ModuleTypeWorkerPool).(common.WorkerPoolModule)
	if !exists {
		return errors.New("worker pool module not found")
	}
	
	pool := workerPoolModule.Pool()
	if pool == nil {
		return errors.New("worker pool is not configured")
	}

	// Create a task function that will be executed by the worker pool
	task := createJobExecutionTask(ctx, s, job)
	
	// Type assert pool to access Submit method
	if submitter, ok := pool.(interface{ Submit(context.Context, func()) error }); ok {
		return submitter.Submit(ctx, task)
	}
	
	return errors.New("worker pool does not support Submit method")
}

// SafeChannelWrite and SafeChannelRead moved to job.go to avoid duplication

// createJobExecutionTask creates a new task function that encapsulates job execution, error handling, and retry logic.
func createJobExecutionTask[T any](ctx context.Context, s *common.Service, job Job[T]) func() {
	return func() {
		// Get logger from service implementation
		log := util.Log(ctx)
		log = log.WithField("job", job.ID()).
			WithField("run", job.Runs())

		if job.F() == nil {
			log.Error("Job function (job.F()) is nil")
			_ = job.WriteError(ctx, errors.New("job function (job.F()) is nil"))
			job.Close()
			return
		}

		job.IncreaseRuns()
		executionErr := job.F()(ctx, job)

		// Handle successful execution first and return early
		if executionErr == nil || errors.Is(executionErr, context.Canceled) {
			job.Close()
			return
		}

		log = log.WithError(executionErr).WithField("can retry", job.CanRun())
		if !job.CanRun() {
			// Job failed and cannot be retried (e.g., retries exhausted).
			log.Error("Job failed; retries exhausted.")
			_ = job.WriteError(ctx, executionErr)
			job.Close()
			return
		}

		// Job can be retried to resolve error
		log.Warn("Job failed, attempting to retry it")
		resubmitErr := SubmitJob(ctx, s, job) // Recursive call to SubmitJob for retry
		if resubmitErr != nil {
			log.WithError(resubmitErr).Error("Failed to resubmit job")
			_ = job.WriteError(ctx, fmt.Errorf("failed to resubmit job: %w", executionErr))
			job.Close()
		}
	}
}
