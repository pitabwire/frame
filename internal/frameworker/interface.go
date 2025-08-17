package frameworker

import (
	"context"
	"time"

	"github.com/pitabwire/util"
)

// WorkerManager defines the interface for worker pool management
type WorkerManager interface {
	// Submit submits a task to the worker pool
	Submit(ctx context.Context, task func()) error
	
	// SubmitJob submits a job to the worker pool for processing
	SubmitJob(ctx context.Context, job Job[any]) error
	
	// Shutdown gracefully shuts down the worker pool
	Shutdown()
}

// WorkerPool defines the common methods for worker pool operations.
// This allows the Service to hold either a single ants.Pool or an ants.MultiPool.
type WorkerPool interface {
	Submit(ctx context.Context, task func()) error
	Shutdown()
}

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

// ConfigurationWorkerPool defines the interface for worker pool configuration
type ConfigurationWorkerPool interface {
	GetCPUFactor() int
	GetCapacity() int
	GetCount() int
	GetExpiryDuration() time.Duration
}

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
