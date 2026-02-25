package workerpool

import (
	"context"
)

const defaultJobResultBufferSize = 10
const defaultJobRetryCount = 0

// JobResult represents the result of a job execution, which can be either a value of type T or an error.
type JobResult[T any] interface {
	IsError() bool
	Error() error
	Item() T
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

type Manager interface {
	GetPool() (WorkerPool, error)
	StopError(context.Context, error)
	Shutdown(context.Context) error
}

// WorkerPool defines the common methods for worker pool operations.
// This allows the Service to hold either a single ants.Pool or an ants.MultiPool.
type WorkerPool interface {
	Submit(ctx context.Context, task func()) error
	Shutdown()
}
