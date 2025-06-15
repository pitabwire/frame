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
	PanicHandler       func(interface{})
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

// WithPoolPanicHandler sets a panic handler for the pool.
func WithPoolPanicHandler(handler func(interface{})) WorkerPoolOption {
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

func defaultWorkerPoolOpts(log *util.LogEntry) *WorkerPoolOptions {
	return &WorkerPoolOptions{
		Concurrency:        runtime.NumCPU() * defaultCPUFactorForWorkerCount,
		SinglePoolCapacity: defaultPoolCapacity,
		PoolCount:          defaultPoolCount,
		ExpiryDuration:     defaultPoolExpiryDuration,
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

type JobResult interface {
	IsError() bool
	Error() error
	Item() any
}

type jobResult struct {
	item  any
	error error
}

func (j *jobResult) IsError() bool {
	return j.error != nil
}

func (j *jobResult) Error() error {
	return j.error
}

func (j *jobResult) Item() any {
	return j.item
}

type JobResultPipe interface {
	ResultBufferSize() int
	ResultChan() <-chan JobResult
	WriteError(ctx context.Context, val error) error
	WriteResult(ctx context.Context, val any) error
	ReadResult(ctx context.Context) (JobResult, bool)
	Close()
}

type Job interface {
	JobResultPipe
	F() func(ctx context.Context, result JobResultPipe) error
	ID() string
	CanRun() bool
	Retries() int
	Runs() int
	IncreaseRuns()
}

type JobImpl struct {
	id               string
	runs             atomic.Int64
	retries          int
	resultBufferSize int
	resultChan       chan JobResult
	resultChanDone   atomic.Bool
	processFunc      func(ctx context.Context, result JobResultPipe) error
}

func (ji *JobImpl) ID() string {
	return ji.id
}

func (ji *JobImpl) F() func(ctx context.Context, result JobResultPipe) error {
	return ji.processFunc
}

func (ji *JobImpl) CanRun() bool {
	return ji.Retries() >= ji.Runs()
}

func (ji *JobImpl) Retries() int {
	return ji.retries
}

func (ji *JobImpl) Runs() int {
	return int(ji.runs.Load())
}

func (ji *JobImpl) IncreaseRuns() {
	ji.runs.Add(1)
}

func (ji *JobImpl) ResultBufferSize() int {
	return ji.resultBufferSize
}

func (ji *JobImpl) ResultChan() <-chan JobResult {
	return ji.resultChan
}

func (ji *JobImpl) ReadResult(ctx context.Context) (JobResult, bool) {
	return SafeChannelRead(ctx, ji.resultChan)
}

func (ji *JobImpl) WriteError(ctx context.Context, val error) error {
	if !ji.resultChanDone.Load() {
		return ErrWorkerPoolResultChannelIsClosed
	}
	util.Log(ctx).Warn("Writing job error", "err", val)
	return SafeChannelWrite(ctx, ji.resultChan, &jobResult{error: val})
}

func (ji *JobImpl) WriteResult(ctx context.Context, val any) error {
	if !ji.resultChanDone.Load() {
		return ErrWorkerPoolResultChannelIsClosed
	}
	util.Log(ctx).Warn("Writing job result", "item", val)
	return SafeChannelWrite(ctx, ji.resultChan, &jobResult{item: val})
}

func (ji *JobImpl) Close() {
	if ji.resultChanDone.CompareAndSwap(false, true) {
		close(ji.resultChan)
	}
}

var _ Job = new(JobImpl)

func NewJob(process func(ctx context.Context, result JobResultPipe) error) Job {
	return NewJobWithBufferAndRetry(process, defaultJobResultBufferSize, defaultJobRetryCount)
}

func NewJobWithBuffer(process func(ctx context.Context, result JobResultPipe) error, buffer int) Job {
	return NewJobWithBufferAndRetry(process, buffer, 0)
}

func NewJobWithRetry(process func(ctx context.Context, result JobResultPipe) error, retries int) Job {
	return NewJobWithBufferAndRetry(process, defaultJobResultBufferSize, retries)
}

func NewJobWithBufferAndRetry(
	process func(ctx context.Context, result JobResultPipe) error,
	resultBufferSize, retries int,
) Job {
	return &JobImpl{
		id:               xid.New().String(),
		retries:          retries,
		resultBufferSize: resultBufferSize,
		resultChan:       make(chan JobResult, resultBufferSize),
		processFunc:      process,
	}
}

// WithBackgroundConsumer sets a background consumer function for the worker pool.
func WithBackgroundConsumer(deque func(_ context.Context) error) Option {
	return func(_ context.Context, s *Service) {
		s.backGroundClient = deque
	}
}

// createJobExecutionTask creates a new task function that encapsulates job execution, error handling, and retry logic.
func createJobExecutionTask(ctx context.Context, s *Service, job Job) func() {
	return func() {
		// At this point, executionErr != nil, so handle the error case.
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
		if executionErr == nil {
			job.Close()
			return
		}

		log = log.WithError(executionErr).WithField("can retry", job.CanRun())
		if !job.CanRun() {
			// Job failed and cannot be retried (e.g., retries exhausted).
			log.Error("Job failed; retries exhausted.")
			_ = job.WriteError(ctx, executionErr)
			job.Close()
		}

		// Job can be retried to resolve error
		log.Warn("Job failed, attempting to retry it")
		resubmitErr := SubmitJob(ctx, s, job) // Recursive call to SubmitJob for retry
		if resubmitErr != nil {
			log.WithError(resubmitErr).
				Error("Failed to resubmit job for retry.")
			// If resubmission fails, the original error of this attempt should be reported.
			_ = job.WriteError(ctx, executionErr)
			job.Close()
		}
	}
}

// SubmitJob used to submit jobs to our worker pool for processing.
// Once a job is submitted the end user does not need to do any further tasks
// One can ideally also wait for the results of their processing for their specific job
// This is done by simply by listening to the jobs ErrChan. Be sure to also check for when its closed
//
//	err, ok := <- errChan
func SubmitJob(ctx context.Context, s *Service, job Job) error {
	// This select block makes the submission attempt itself cancellable by ctx.
	// It attempts to submit once.
	select {
	case <-ctx.Done():
		// Use ctx.Err() to provide a more specific error about context cancellation.
		return fmt.Errorf("context cancelled before job submission: %w", ctx.Err())
	default:

		// Create the actual task to be executed by a worker.
		task := createJobExecutionTask(ctx, s, job)
		return s.pool.Submit(ctx, task)
	}
}

// SafeChannelWrite writes a value to a channel, returning an error if the context is canceled.
func SafeChannelWrite(ctx context.Context, ch chan<- JobResult, value JobResult) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- value:
		return nil
	}
}

func SafeChannelRead(ctx context.Context, ch <-chan JobResult) (JobResult, bool) {
	select {
	case <-ctx.Done():
		// Return context error without blocking
		return &jobResult{error: ctx.Err()}, false

	case result, ok := <-ch:
		// Channel read successfully or channel closed
		return result, ok
	}
}
