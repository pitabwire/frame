package frame

import (
	"context"
	"errors"
	"github.com/rs/xid"
)

type Job interface {
	Process(ctx context.Context) error
	ID() string
	CanRetry() bool
	DecreaseRetries() int
}

type JobImpl struct {
	Id      string
	Retries int
	process func(ctx context.Context) error
}

func (ji *JobImpl) ID() string {
	return ji.Id
}

func (ji *JobImpl) Process(ctx context.Context) error {
	if ji.process != nil {
		return ji.process(ctx)
	}
	return errors.New("implement this function")
}
func (ji *JobImpl) CanRetry() bool {
	return ji.Retries > 0
}
func (ji *JobImpl) DecreaseRetries() int {
	ji.Retries = ji.Retries - 1
	return ji.Retries
}

func (s *Service) NewJob(process func(ctx context.Context) error, retries int) Job {
	return &JobImpl{
		Id:      xid.New().String(),
		Retries: retries,
		process: process,
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
					logger := s.L().WithError(err).WithField("job", job.ID())
					logger.Error("could not process job")

					if job.CanRetry() {
						job.DecreaseRetries()
						err = s.SubmitJob(ctx, job)
						if err != nil {
							logger.Error("could not resubmit job for retry")
						} else {
							logger.Info("job resubmitted for retry")
						}
					}

					return
				}

			},
		)
		return nil
	}
}
