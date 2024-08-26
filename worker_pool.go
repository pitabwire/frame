package frame

import (
	"context"
	"errors"
	"github.com/rs/xid"
	"runtime/debug"
	"sync"
)

type JobResultPipe interface {
	ResultBufferSize() int
	ResultChan() <-chan any
	WriteResult(ctx context.Context, val any) error
	ReadResult(ctx context.Context) (any, bool, error)
	IsClosed() bool
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
	runs             int
	retries          int
	resultBufferSize int
	resultChan       chan any
	resultMu         sync.Mutex
	resultChanDone   bool
	processFunc      func(ctx context.Context, result JobResultPipe) error
}

func (ji *JobImpl) ID() string {
	return ji.id
}

func (ji *JobImpl) F() func(ctx context.Context, result JobResultPipe) error {
	return ji.processFunc
}

func (ji *JobImpl) CanRun() bool {
	return ji.Retries() > (ji.Runs() - 1)
}

func (ji *JobImpl) Retries() int {
	return ji.retries
}

func (ji *JobImpl) Runs() int {
	return ji.runs
}

func (ji *JobImpl) IncreaseRuns() {
	ji.resultMu.Lock()
	defer ji.resultMu.Unlock()

	ji.runs = ji.runs + 1
}

func (ji *JobImpl) ResultBufferSize() int {
	return ji.resultBufferSize
}

func (ji *JobImpl) ResultChan() <-chan any {
	return ji.resultChan
}

func (ji *JobImpl) ReadResult(ctx context.Context) (any, bool, error) {
	return SafeChannelRead(ctx, ji.resultChan)
}
func (ji *JobImpl) WriteResult(ctx context.Context, val any) error {
	return SafeChannelWrite(ctx, ji.resultChan, val)
}

func (ji *JobImpl) Close() {

	ji.resultMu.Lock()
	defer ji.resultMu.Unlock()
	if !ji.resultChanDone {
		close(ji.resultChan)
		ji.resultChanDone = true
	}
}

func (ji *JobImpl) IsClosed() bool {
	ji.resultMu.Lock()
	defer ji.resultMu.Unlock()
	return ji.resultChanDone
}

func (s *Service) NewJob(process func(ctx context.Context, result JobResultPipe) error) Job {
	return s.NewJobWithBufferAndRetry(process, 10, 0)
}

func (s *Service) NewJobWithBuffer(process func(ctx context.Context, result JobResultPipe) error, buffer int) Job {
	return s.NewJobWithBufferAndRetry(process, buffer, 0)
}

func (s *Service) NewJobWithRetry(process func(ctx context.Context, result JobResultPipe) error, retries int) Job {
	return s.NewJobWithBufferAndRetry(process, 10, retries)
}

func (s *Service) NewJobWithBufferAndRetry(process func(ctx context.Context, result JobResultPipe) error, resultBufferSize, retries int) Job {
	return &JobImpl{
		id:               xid.New().String(),
		retries:          retries,
		processFunc:      process,
		resultBufferSize: resultBufferSize,
		resultChan:       make(chan any, resultBufferSize),
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
func (s *Service) SubmitJob(ctx context.Context, job Job) error {

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
						err := job.WriteResult(ctx, errors.New("implement this function"))
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

							err1 := s.SubmitJob(ctx, job)
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
func SafeChannelWrite(ctx context.Context, ch chan<- any, value any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- value:
		return nil
	}
}

func SafeChannelRead(ctx context.Context, ch <-chan any) (any, bool, error) {

	select {
	case <-ctx.Done():
		// If the context is canceled, drain the channel in a non-blocking way
		go func() {
			for range ch {
				// Draining the channel
			}
		}()
		// Return context error without blocking
		return nil, false, ctx.Err()

	case result, ok := <-ch:
		// Channel read successfully or channel closed
		return result, ok, nil
	}
}
