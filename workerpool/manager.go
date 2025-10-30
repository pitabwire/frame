package workerpool

import (
	"context"
	"errors"
	"fmt"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/util"
)

type manager struct {
	pool    WorkerPool
	stopErr func(ctx context.Context, err error)
}

func NewManager(
	ctx context.Context,
	cfg config.ConfigurationWorkerPool,
	stopOnErr func(ctx context.Context, err error),
	opts ...Option,
) Manager {
	log := util.Log(ctx)

	poolOpts := defaultWorkerPoolOpts(cfg, log)

	for _, opt := range opts {
		opt(poolOpts)
	}

	pool, err := setupWorkerPool(ctx, poolOpts)
	if err != nil {
		log.WithError(err).Panic("could not create a default worker pool")
	}

	return &manager{
		pool:    pool,
		stopErr: stopOnErr,
	}
}

func (m manager) GetPool() (WorkerPool, error) {
	if m.pool == nil {
		return nil, errors.New("worker pool is not configured")
	}
	return m.pool, nil
}

func (m manager) StopError(ctx context.Context, err error) {
	m.stopErr(ctx, err)
}

// SubmitJob used to submit jobs to our worker pool for processing.
// Once a job is submitted the end user does not need to do any further tasks
// One can ideally also wait for the results of their processing for their specific job
// by listening to the job's ResultChan.
func SubmitJob[T any](ctx context.Context, m Manager, job Job[T]) error {
	if m == nil {
		return errors.New("service is nil")
	}

	pool, err := m.GetPool()
	if err != nil {
		return err
	}

	// Create a task function that will be executed by the worker pool
	task := createJobExecutionTask(ctx, m, job)
	return pool.Submit(ctx, task)
}

// createJobExecutionTask creates a new task function that encapsulates job execution, error handling, and retry logic.
func createJobExecutionTask[T any](ctx context.Context, s Manager, job Job[T]) func() {
	return func() {
		log := util.Log(ctx).
			WithField("job", job.ID()).
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
