package frame

import (
	"context"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/workerpool"
)

// WithBackgroundConsumer sets a background consumer function for the worker pool.
func WithBackgroundConsumer(deque func(_ context.Context) error) Option {
	return func(_ context.Context, s *Service) {
		s.backGroundClient = deque
	}
}

// WithWorkerPoolOptions provides a way to set custom options for the ants worker pool.
// Renamed from WithAntsOptions and changed parameter type.
func WithWorkerPoolOptions(options ...workerpool.Option) Option {
	return func(ctx context.Context, s *Service) {
		cfg, ok := s.Config().(config.ConfigurationWorkerPool)
		if !ok {
			s.Log(ctx).Error("worker pool configuration is not setup")
			return
		}

		wpm, err := workerpool.NewManager(ctx, cfg, s.sendStopError, options...)
		if err != nil {
			s.AddStartupError(err)
			return
		}
		s.workerPoolManager = wpm
	}
}

func (s *Service) WorkManager() workerpool.Manager {
	return s.workerPoolManager
}

// SubmitJob used to submit jobs to our worker pool for processing.
// Once a job is submitted the end user does not need to do any further tasks
// One can ideally also wait for the results of their processing for their specific job
// by listening to the job's ResultChan.
func SubmitJob[T any](ctx context.Context, s *Service, job workerpool.Job[T]) error {
	return workerpool.SubmitJob(ctx, s.WorkManager(), job)
}
