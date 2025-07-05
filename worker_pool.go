package frame

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/pitabwire/util"
	"github.com/rs/xid"
)

const (
	defaultCPUFactorForWorkerCount = 10
	defaultPoolCapacity            = 100
	defaultPoolCount               = 1
	defaultPoolExpiryDuration      = 1 * time.Second
)

var ErrWorkerPoolResultChannelIsClosed = errors.New("worker job is already closed")

// WorkerPoolOptions defines configurable options for the service's internal worker pool.
type WorkerPoolOptions struct {
	PoolCount          int
	SinglePoolCapacity int
	Concurrency        int
	ExpiryDuration     time.Duration
	Nonblocking        bool
	PreAlloc           bool
	PanicHandler       func(any)
	Logger             *util.LogEntry
	DisablePurge       bool
}

// WorkerPoolOption defines a function that configures worker pool options.
type WorkerPoolOption func(*WorkerPoolOptions)

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

// WithWorkerPoolOptions provides a way to set custom options for the ants worker pool.
// Renamed from WithAntsOptions and changed parameter type.
func WithWorkerPoolOptions(options ...WorkerPoolOption) Option {
	return func(ctx context.Context, s *Service) {
		for _, opt := range options {
			opt(s.poolOptions)
		}

		log := util.Log(ctx)

		if s.pool != nil {
			s.pool.Shutdown()
			s.pool = nil
		}

		var err error
		s.pool, err = setupWorkerPool(ctx, s.poolOptions)
		if err != nil {
			log.WithError(err).Panic("could not create a default worker pool")
		}
	}
}

// WorkerPool defines the common methods for worker pool operations.
// This allows the Service to hold either a single ants.Pool or an ants.MultiPool.
type WorkerPool interface {
	Submit(ctx context.Context, task func()) error
	Shutdown()
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

func setupWorkerPool(_ context.Context, wopts *WorkerPoolOptions) (WorkerPool, error) {
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

const defaultJobResultBufferSize = 10
const defaultJobRetryCount = 0

// JobResult represents the result of a job execution, which can be either a value of type T or an error.
type JobResult[T any] interface {
	IsError() bool
	Error() error
	Item() T
}

// jobResult is the internal implementation of JobResult.
type jobResult[T any] struct {
	item  T
	error error
}

func (j *jobResult[T]) IsError() bool {
	return j.error != nil
}

func (j *jobResult[T]) Error() error {
	return j.error
}

func (j *jobResult[T]) Item() T {
	return j.item
}

// JobResultPipe is a channel-based pipeline for passing job results.
type JobResultPipe[T any] interface {
	ResultBufferSize() int
	ResultChan() <-chan JobResult[T]
	WriteError(ctx context.Context, val error) error
	WriteResult(ctx context.Context, val T) error
	ReadResult(ctx context.Context) (JobResult[T], bool)
	Close()
}

// Job represents a task that can be executed and produce results of type T.
type Job[T any] interface {
	JobResultPipe[T]
	F() func(ctx context.Context, result JobResultPipe[T]) error
	ID() string
	CanRun() bool
	Retries() int
	Runs() int
	IncreaseRuns()
}

// JobImpl is the concrete implementation of a Job.
type JobImpl[T any] struct {
	id               string
	runs             atomic.Int64
	retries          int
	resultBufferSize int
	resultChan       chan JobResult[T]
	resultChanDone   atomic.Bool
	processFunc      func(ctx context.Context, result JobResultPipe[T]) error
}

func (ji *JobImpl[T]) ID() string {
	return ji.id
}

func (ji *JobImpl[T]) F() func(ctx context.Context, result JobResultPipe[T]) error {
	return ji.processFunc
}

func (ji *JobImpl[T]) CanRun() bool {
	return ji.Retries() >= ji.Runs()
}

func (ji *JobImpl[T]) Retries() int {
	return ji.retries
}

func (ji *JobImpl[T]) Runs() int {
	return int(ji.runs.Load())
}

func (ji *JobImpl[T]) IncreaseRuns() {
	ji.runs.Add(1)
}

func (ji *JobImpl[T]) ResultBufferSize() int {
	return ji.resultBufferSize
}

func (ji *JobImpl[T]) ResultChan() <-chan JobResult[T] {
	return ji.resultChan
}

func (ji *JobImpl[T]) ReadResult(ctx context.Context) (JobResult[T], bool) {
	return SafeChannelRead(ctx, ji.resultChan)
}

func (ji *JobImpl[T]) WriteError(ctx context.Context, val error) error {
	if ji.resultChanDone.Load() {
		return ErrWorkerPoolResultChannelIsClosed
	}
	return SafeChannelWrite(ctx, ji.resultChan, &jobResult[T]{error: val})
}

func (ji *JobImpl[T]) WriteResult(ctx context.Context, val T) error {
	if ji.resultChanDone.Load() {
		return ErrWorkerPoolResultChannelIsClosed
	}
	return SafeChannelWrite(ctx, ji.resultChan, &jobResult[T]{item: val})
}

func (ji *JobImpl[T]) Close() {
	if ji.resultChanDone.CompareAndSwap(false, true) {
		close(ji.resultChan)
	}
}

// NewJob creates a new job with default buffer size and retry count.
func NewJob[T any](process func(ctx context.Context, result JobResultPipe[T]) error) Job[T] {
	return NewJobWithBufferAndRetry[T](process, defaultJobResultBufferSize, defaultJobRetryCount)
}

// NewJobWithBuffer creates a new job with a specified buffer size.
func NewJobWithBuffer[T any](process func(ctx context.Context, result JobResultPipe[T]) error, buffer int) Job[T] {
	return NewJobWithBufferAndRetry[T](process, buffer, 0)
}

// NewJobWithRetry creates a new job with a specified retry count.
func NewJobWithRetry[T any](process func(ctx context.Context, result JobResultPipe[T]) error, retries int) Job[T] {
	return NewJobWithBufferAndRetry[T](process, defaultJobResultBufferSize, retries)
}

// NewJobWithBufferAndRetry creates a new job with specified buffer size and retry count.
func NewJobWithBufferAndRetry[T any](
	process func(ctx context.Context, result JobResultPipe[T]) error,
	resultBufferSize, retries int,
) Job[T] {
	return &JobImpl[T]{
		id:               xid.New().String(),
		retries:          retries,
		resultBufferSize: resultBufferSize,
		resultChan:       make(chan JobResult[T], resultBufferSize),
		processFunc:      process,
	}
}

// WithBackgroundConsumer sets a background consumer function for the worker pool.
func WithBackgroundConsumer(deque func(_ context.Context) error) Option {
	return func(_ context.Context, s *Service) {
		s.backGroundClient = deque
	}
}

// SubmitJob used to submit jobs to our worker pool for processing.
// Once a job is submitted the end user does not need to do any further tasks
// One can ideally also wait for the results of their processing for their specific job
// by listening to the job's ResultChan.
func SubmitJob[T any](ctx context.Context, s *Service, job Job[T]) error {
	if s == nil {
		return errors.New("service is nil")
	}

	if s.pool == nil {
		return errors.New("worker pool is not configured")
	}

	// Create a task function that will be executed by the worker pool
	task := createJobExecutionTask(ctx, s, job)
	return s.pool.Submit(ctx, task)
}

// SafeChannelWrite writes a value to a channel, returning an error if the context is canceled.
func SafeChannelWrite[T any](ctx context.Context, ch chan<- JobResult[T], value JobResult[T]) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled while writing to channel: %w", ctx.Err())
	case ch <- value:
		return nil
	}
}

// SafeChannelRead reads a value from a channel, returning false if the channel is closed or the context is canceled.
func SafeChannelRead[T any](ctx context.Context, ch <-chan JobResult[T]) (JobResult[T], bool) {
	select {
	case <-ctx.Done():
		var zero JobResult[T]
		return zero, false
	case result, ok := <-ch:
		return result, ok
	}
}

// createJobExecutionTask creates a new task function that encapsulates job execution, error handling, and retry logic.
func createJobExecutionTask[T any](ctx context.Context, s *Service, job Job[T]) func() {
	return func() {
		log := s.Log(ctx).
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
