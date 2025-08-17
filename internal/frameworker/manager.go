package frameworker

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/pitabwire/util"
)

var ErrWorkerPoolResultChannelIsClosed = errors.New("worker job is already closed")

const defaultJobResultBufferSize = 10
const defaultJobRetryCount = 0

// Manager implements the WorkerManager interface
type Manager struct {
	pool               WorkerPool
	options            *WorkerPoolOptions
	backgroundConsumer func(context.Context) error
	logger             *util.LogEntry
}

// NewManager creates a new worker manager
func NewManager(options *WorkerPoolOptions) (*Manager, error) {
	pool, err := setupWorkerPool(options)
	if err != nil {
		return nil, err
	}

	return &Manager{
		pool:    pool,
		options: options,
		logger:  options.Logger,
	}, nil
}

// NewManagerWithDefaults creates a new worker manager with default configuration
func NewManagerWithDefaults(cfg ConfigurationWorkerPool, logger *util.LogEntry) (*Manager, error) {
	options := defaultWorkerPoolOpts(cfg, logger)
	return NewManager(options)
}

// Submit submits a task to the worker pool
func (m *Manager) Submit(ctx context.Context, task func()) error {
	if m.pool == nil {
		return errors.New("worker pool is not configured")
	}
	return m.pool.Submit(ctx, task)
}

// SubmitJob submits a job to the worker pool for processing
func (m *Manager) SubmitJob(ctx context.Context, job Job[any]) error {
	if m.pool == nil {
		return errors.New("worker pool is not configured")
	}

	// Create a task function that will be executed by the worker pool
	task := m.createJobExecutionTask(ctx, job)
	return m.pool.Submit(ctx, task)
}

// Shutdown gracefully shuts down the worker pool
func (m *Manager) Shutdown() {
	if m.pool != nil {
		m.pool.Shutdown()
	}
}

// SetBackgroundConsumer sets a background consumer function
func (m *Manager) SetBackgroundConsumer(consumer func(context.Context) error) {
	m.backgroundConsumer = consumer
}

// GetBackgroundConsumer returns the background consumer function
func (m *Manager) GetBackgroundConsumer() func(context.Context) error {
	return m.backgroundConsumer
}

func defaultWorkerPoolOpts(cfg ConfigurationWorkerPool, log *util.LogEntry) *WorkerPoolOptions {
	return &WorkerPoolOptions{
		Concurrency:        runtime.NumCPU() * cfg.GetCPUFactor(),
		SinglePoolCapacity: cfg.GetCapacity(),
		PoolCount:          cfg.GetCount(),
		ExpiryDuration:     cfg.GetExpiryDuration(),
		Nonblocking:        true,
		PreAlloc:           false,
		PanicHandler:       nil,
		Logger:             log,
		DisablePurge:       false,
	}
}

func setupWorkerPool(wopts *WorkerPoolOptions) (WorkerPool, error) {
	var antsOpts []ants.Option
	if wopts.ExpiryDuration > 0 {
		antsOpts = append(antsOpts, ants.WithExpiryDuration(wopts.ExpiryDuration))
	}
	antsOpts = append(antsOpts, ants.WithNonblocking(wopts.Nonblocking))
	if wopts.PreAlloc {
		antsOpts = append(antsOpts, ants.WithPreAlloc(wopts.PreAlloc))
	}
	if wopts.Concurrency > 0 {
		antsOpts = append(antsOpts, ants.WithMaxBlockingTasks(wopts.Concurrency))
	}
	if wopts.PanicHandler != nil {
		antsOpts = append(antsOpts, ants.WithPanicHandler(wopts.PanicHandler))
	}

	antsOpts = append(antsOpts, ants.WithLogger(wopts.Logger))
	antsOpts = append(antsOpts, ants.WithDisablePurge(wopts.DisablePurge))

	var err error

	if wopts.PoolCount == 1 {
		var p *ants.Pool
		p, err = ants.NewPool(wopts.SinglePoolCapacity, antsOpts...)
		if err != nil {
			return nil, err
		}
		return &singlePoolWrapper{pool: p}, nil
	}

	var mp *ants.MultiPool
	mp, err = ants.NewMultiPool(wopts.PoolCount, wopts.SinglePoolCapacity, ants.LeastTasks, antsOpts...)
	if err != nil {
		return nil, err
	}
	return &multiPoolWrapper{multiPool: mp}, nil
}

// singlePoolWrapper adapts *ants.Pool to the WorkerPool interface.
type singlePoolWrapper struct {
	pool *ants.Pool
}

func (w *singlePoolWrapper) Submit(ctx context.Context, task func()) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return w.pool.Submit(task)
}

func (w *singlePoolWrapper) Shutdown() {
	w.pool.Release()
}

// multiPoolWrapper adapts *ants.MultiPool to the WorkerPool interface.
type multiPoolWrapper struct {
	multiPool *ants.MultiPool
}

func (w *multiPoolWrapper) Submit(ctx context.Context, task func()) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return w.multiPool.Submit(task)
}

func (w *multiPoolWrapper) Shutdown() {
	w.multiPool.Free()
}

// createJobExecutionTask creates a new task function that encapsulates job execution, error handling, and retry logic.
func (m *Manager) createJobExecutionTask(ctx context.Context, job Job[any]) func() {
	return func() {
		log := m.logger.WithContext(ctx).
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
		resubmitErr := m.SubmitJob(ctx, job) // Recursive call to SubmitJob for retry
		if resubmitErr != nil {
			log.WithError(resubmitErr).Error("Failed to resubmit job")
			_ = job.WriteError(ctx, fmt.Errorf("failed to resubmit job: %w", executionErr))
			job.Close()
		}
	}
}

// Worker pool option functions

// WithPoolCount sets the number of worker pools.
func WithPoolCount(count int) WorkerPoolOption {
	return func(opts *WorkerPoolOptions) {
		opts.PoolCount = count
	}
}

// WithSinglePoolCapacity sets the capacity for a single worker pool.
func WithSinglePoolCapacity(capacity int) WorkerPoolOption {
	return func(opts *WorkerPoolOptions) {
		opts.SinglePoolCapacity = capacity
	}
}

// WithConcurrency sets the concurrency for the worker pool.
func WithConcurrency(concurrency int) WorkerPoolOption {
	return func(opts *WorkerPoolOptions) {
		opts.Concurrency = concurrency
	}
}

// WithPoolExpiryDuration sets the expiry duration for workers.
func WithPoolExpiryDuration(duration time.Duration) WorkerPoolOption {
	return func(opts *WorkerPoolOptions) {
		opts.ExpiryDuration = duration
	}
}

// WithPoolNonblocking sets the non-blocking option for the pool.
func WithPoolNonblocking(nonblocking bool) WorkerPoolOption {
	return func(opts *WorkerPoolOptions) {
		opts.Nonblocking = nonblocking
	}
}

// WithPoolPreAlloc pre-allocates memory for the pool.
func WithPoolPreAlloc(preAlloc bool) WorkerPoolOption {
	return func(opts *WorkerPoolOptions) {
		opts.PreAlloc = preAlloc
	}
}

// WithPoolPanicHandler sets a panic handlers for the pool.
func WithPoolPanicHandler(handler func(any)) WorkerPoolOption {
	return func(opts *WorkerPoolOptions) {
		opts.PanicHandler = handler
	}
}

// WithPoolLogger sets a logger for the pool.
func WithPoolLogger(logger *util.LogEntry) WorkerPoolOption {
	return func(opts *WorkerPoolOptions) {
		opts.Logger = logger
	}
}

// WithPoolDisablePurge disables the purge mechanism in the pool.
func WithPoolDisablePurge(disable bool) WorkerPoolOption {
	return func(opts *WorkerPoolOptions) {
		opts.DisablePurge = disable
	}
}
