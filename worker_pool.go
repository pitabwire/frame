package frame

import (
	"context"
	"errors"
	"github.com/rs/xid"
	"runtime/debug"
)

type Job interface {
	F() func(ctx context.Context) error
	Process(ctx context.Context) error
	ID() string
	CanRetry() bool
	DecreaseRetries() int
	ErrChan() chan error
	PipeError(err error)
	CloseChan()
}

type JobImpl struct {
	id        string
	retries   int
	errorChan chan error

	processFunc func(ctx context.Context) error
}

func (ji *JobImpl) ID() string {
	return ji.id
}

func (ji *JobImpl) F() func(ctx context.Context) error {
	return ji.processFunc
}

func (ji *JobImpl) Process(ctx context.Context) error {
	if ji.processFunc != nil {
		return ji.processFunc(ctx)
	}
	return errors.New("implement this function")
}
func (ji *JobImpl) CanRetry() bool {
	return ji.retries > 0
}

func (ji *JobImpl) DecreaseRetries() int {
	ji.retries = ji.retries - 1
	return ji.retries
}
func (ji *JobImpl) ErrChan() chan error {
	return ji.errorChan
}
func (ji *JobImpl) PipeError(err error) {
	if ji.ErrChan() != nil {
		select {
		case ji.errorChan <- err:
			break
		default:
		}

	}
	ji.CloseChan()
}
func (ji *JobImpl) CloseChan() {
	if ji.ErrChan() != nil {
		close(ji.errorChan)
	}
}

func (s *Service) NewJob(process func(ctx context.Context) error) Job {
	return s.NewJobWithRetry(process, 0)
}

func (s *Service) NewJobWithRetry(process func(ctx context.Context) error, retries int) Job {
	return s.NewJobWithRetryAndErrorChan(process, retries, nil)
}

func (s *Service) NewJobWithRetryAndErrorChan(process func(ctx context.Context) error, retries int, errChan chan error) Job {
	return &JobImpl{
		id:          xid.New().String(),
		retries:     retries,
		processFunc: process,
		errorChan:   errChan,
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
	if p.Stopped() {
		return errors.New("pool is closed")
	}
	select {
	case <-ctx.Done():
		return errors.New("pool is closed")
	default:

		p.Submit(
			func() {

				err := job.Process(ctx)
				if err != nil {
					logger := s.L().WithError(err).
						WithField("job", job.ID())

					if !job.CanRetry() {

						logger.
							WithField("stacktrace", string(debug.Stack())).
							Error("could not processFunc job")

						job.PipeError(err)
						return
					}
					retries := job.DecreaseRetries()

					retryJob := s.NewJobWithRetryAndErrorChan(job.F(), retries, job.ErrChan())
					err1 := s.SubmitJob(ctx, retryJob)
					if err1 != nil {
						logger.
							WithField("stacktrace", string(debug.Stack())).
							Error("could not resubmit job for retry")
						job.PipeError(err)
					} else {
						logger.Info("job resubmitted for retry")
						return
					}

				}

				// Return a nil just to make sure if a processFunc is waiting on the channel we can have it exit
				job.CloseChan()
			},
		)
		return nil
	}
}
