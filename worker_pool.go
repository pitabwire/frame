package frame

import (
	"context"
	"errors"
	"github.com/rs/xid"
	"runtime/debug"
	"sync"
)

type JobResult[J any] struct {
	Item  J
	Error error
}

func (j JobResult[J]) IsError() bool {
	return j.Error != nil
}

type JobResultPipe[J any] interface {
	ResultBufferSize() int
	ResultChan() <-chan JobResult[J]
	WriteResult(ctx context.Context, val JobResult[J]) error
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

	ji.runs = ji.runs + 1
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
func (ji *JobImpl[J]) WriteResult(ctx context.Context, val JobResult[J]) error {
	return SafeChannelWrite(ctx, ji.resultChan, val)
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
	return NewJobWithBufferAndRetry[J](process, 10, 0)
}

func NewJobWithBuffer[J any](process func(ctx context.Context, result JobResultPipe[J]) error, buffer int) Job[J] {
	return NewJobWithBufferAndRetry(process, buffer, 0)
}

func NewJobWithRetry[J any](process func(ctx context.Context, result JobResultPipe[J]) error, retries int) Job[J] {
	return NewJobWithBufferAndRetry(process, 10, retries)
}

func NewJobWithBufferAndRetry[J any](process func(ctx context.Context, result JobResultPipe[J]) error, resultBufferSize, retries int) Job[J] {
	return &JobImpl[J]{
		id:               xid.New().String(),
		retries:          retries,
		processFunc:      process,
		resultBufferSize: resultBufferSize,
		resultChan:       make(chan JobResult[J], resultBufferSize),
	}
}

// BackGroundConsumer Option to register a background processing function that is initialized before running servers
// this function is maintained alive using the same error group as the servers so that if any exit earlier due to error
// all stop functioning
func BackGroundConsumer(deque func(ctx context.Context) error) Option {
	return func(s *Service) {
		s.backGroundClient = deque
	}
}

// WithPoolConcurrency Option sets the count of pool workers to handle server load.
// By default this is count of CPU + 1
func WithPoolConcurrency(workers int) Option {
	return func(s *Service) {
		s.poolWorkerCount = workers
	}
}

// WithPoolCapacity Option sets the capacity of pool workers to handle server load.
// By default this is 100
func WithPoolCapacity(capacity int) Option {
	return func(s *Service) {
		s.poolCapacity = capacity
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

	for {
		select {
		case <-ctx.Done():
			return errors.New("pool is closed")
		default:

			if !job.CanRun() {
				return nil
			}

			return p.Submit(
				func() {

					defer job.Close()

					if job.F() == nil {
						err := job.WriteResult(ctx, JobResult[J]{Error: errors.New("implement this function")})
						if err != nil {
							return
						}
						return
					}

					job.IncreaseRuns()
					err := job.F()(ctx, job)
					if err != nil {
						logger := s.L(ctx).WithError(err).
							WithField("job", job.ID()).
							WithField("retry", job.Retries())

						if job.CanRun() {

							err1 := SubmitJob(ctx, s, job)
							if err1 != nil {
								logger.
									WithError(err1).
									WithField("stacktrace", string(debug.Stack())).
									Info("could not resubmit job for retry")
								return
							} else {
								logger.Debug("job resubmitted for retry")
								return
							}
						}
					}
				},
			)
		}
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
		return JobResult[J]{Error: ctx.Err()}, false

	case result, ok := <-ch:
		// Channel read successfully or channel closed
		return result, ok
	}
}
