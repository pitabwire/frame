package frameworker

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/rs/xid"
)

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

// Result creates a successful job result
func Result[T any](item T) JobResult[T] {
	return &jobResult[T]{item: item}
}

// ErrorResult creates an error job result
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
