package frame

import (
	"context"
	"errors"
	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
	"sync"
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

// WithConcurrency Option sets the count of pool workers to handle server load.
// By default this is count of CPU + 1
func WithConcurrency(workers int) Option {
	return func(s *Service) {
		s.workerCount = workers
	}
}

type worker struct {
	id        string
	jobQueues chan Job
	logger    *logrus.Entry
	wg        *sync.WaitGroup
}

func newWorker(logger *logrus.Entry, jobQueue chan Job, wg *sync.WaitGroup) *worker {
	workerId := xid.New().String()
	return &worker{
		id:        workerId,
		jobQueues: jobQueue,
		wg:        wg,
		logger:    logger.WithField("worker", workerId),
	}
}

func (w *worker) start(ctx context.Context) {
	defer w.wg.Done()
	for {
		select {
		case job, ok := <-w.jobQueues:
			if !ok {
				// job queue closed, quit the worker
				return
			}
			// process the job
			if err := job.Process(ctx); err != nil {

				log := w.logger.WithError(err).WithField("id", job.ID())
				// if job has retries left, add it back to job queue with retries reduced

				if job.CanRetry() {
					availableRetries := job.DecreaseRetries()
					select {
					case w.jobQueues <- job:
						log.WithField("available retries", availableRetries).Info("successfully requeued job")
					case <-ctx.Done():
						return
					}
					continue
				} else {
					log.Error("failed to process job")
				}
			}
		case <-ctx.Done():
			// quit the worker
			return
		}
	}
}

type pool struct {
	Workers   []*worker
	jobQueue  chan Job
	wg        sync.WaitGroup
	isClosed  bool
	closeOnce sync.Once
	mutex     sync.Mutex
}

func (p *pool) Close() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.closeOnce.Do(func() {
		p.isClosed = true
		close(p.jobQueue)
	})
	return nil
}

func (p *pool) Start(ctx context.Context) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.jobQueue = make(chan Job)

	for i := 0; i < len(p.Workers); i++ {
		p.wg.Add(1)
		go p.Workers[i].start(ctx)
	}

}

func newPool(logger *logrus.Entry, numWorkers int) *pool {
	jobQueue := make(chan Job)
	pl := &pool{
		Workers:  make([]*worker, numWorkers),
		jobQueue: jobQueue,
	}

	for i := 0; i < numWorkers; i++ {
		wrk := newWorker(logger, pl.jobQueue, &pl.wg)
		pl.Workers[i] = wrk
	}
	return pl
}

func (s *Service) SubmitJob(ctx context.Context, job Job) error {

	p := s.pool
	if p.isClosed {
		return errors.New("pool is closed")
	}
	select {
	case p.jobQueue <- job:
		return nil
	case <-ctx.Done():
		return errors.New("pool is closed")
	}
}
