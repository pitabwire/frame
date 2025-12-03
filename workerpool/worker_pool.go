package workerpool

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

	"github.com/pitabwire/frame/config"
)

var ErrWorkerPoolResultChannelIsClosed = errors.New("worker job is already closed")

// Options defines configurable options for the service's internal worker pool.
type Options struct {
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

// Option defines a function that configures worker pool options.
type Option func(*Options)

// WithPoolCount sets the number of worker pools.
func WithPoolCount(count int) Option {
	return func(opts *Options) {
		opts.PoolCount = count
	}
}

// WithSinglePoolCapacity sets the capacity for a single worker pool.
func WithSinglePoolCapacity(capacity int) Option {
	return func(opts *Options) {
		opts.SinglePoolCapacity = capacity
	}
}

// WithConcurrency sets the concurrency for the worker pool.
func WithConcurrency(concurrency int) Option {
	return func(opts *Options) {
		opts.Concurrency = concurrency
	}
}

// WithPoolExpiryDuration sets the expiry duration for workers.
func WithPoolExpiryDuration(duration time.Duration) Option {
	return func(opts *Options) {
		opts.ExpiryDuration = duration
	}
}

// WithPoolNonblocking sets the non-blocking option for the pool.
func WithPoolNonblocking(nonblocking bool) Option {
	return func(opts *Options) {
		opts.Nonblocking = nonblocking
	}
}

// WithPoolPreAlloc pre-allocates memory for the pool.
func WithPoolPreAlloc(preAlloc bool) Option {
	return func(opts *Options) {
		opts.PreAlloc = preAlloc
	}
}

// WithPoolPanicHandler sets a panic handlers for the pool.
func WithPoolPanicHandler(handler func(any)) Option {
	return func(opts *Options) {
		opts.PanicHandler = handler
	}
}

// WithPoolLogger sets a logger for the pool.
func WithPoolLogger(logger *util.LogEntry) Option {
	return func(opts *Options) {
		opts.Logger = logger
	}
}

// WithPoolDisablePurge disables the purge mechanism in the pool.
func WithPoolDisablePurge(disable bool) Option {
	return func(opts *Options) {
		opts.DisablePurge = disable
	}
}

func defaultWorkerPoolOpts(cfg config.ConfigurationWorkerPool, log *util.LogEntry) *Options {
	return &Options{
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

func setupWorkerPool(_ context.Context, wopts *Options) (WorkerPool, error) {
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

	if wopts.PoolCount <= 1 {
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

func Result[T any](item T) JobResult[T] {
	return &jobResult[T]{item: item}
}

func ErrorResult[T any](err error) JobResult[T] {
	return &jobResult[T]{error: err}
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
	return SafeChannelWrite(ctx, ji.resultChan, ErrorResult[T](val))
}

func (ji *JobImpl[T]) WriteResult(ctx context.Context, val T) error {
	if ji.resultChanDone.Load() {
		return ErrWorkerPoolResultChannelIsClosed
	}
	return SafeChannelWrite(ctx, ji.resultChan, Result[T](val))
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

// SafeChannelWrite writes a value to a channel, returning an error if the context is canceled.
func SafeChannelWrite[T any](ctx context.Context, ch chan<- JobResult[T], value JobResult[T]) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled while writing to channel: %w", ctx.Err())
	default:
	}

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
	default:
	}

	select {
	case <-ctx.Done():
		var zero JobResult[T]
		return zero, false
	case result, ok := <-ch:
		return result, ok
	}
}

func ConsumeResultStream[T any](ctx context.Context, job JobResultPipe[T], consumer func(T)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:

			res, ok := job.ReadResult(ctx)
			if !ok {
				return nil
			}

			if res.IsError() {
				return res.Error()
			}

			consumer(res.Item())
		}
	}
}
