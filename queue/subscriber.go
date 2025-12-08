package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pitabwire/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"gocloud.dev/pubsub"

	"github.com/pitabwire/frame/localization"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/workerpool"
)

type subscriber struct {
	reference    string
	url          string
	handlers     []SubscribeWorker
	subscription *pubsub.Subscription
	isInit       atomic.Bool
	state        SubscriberState
	metrics      *subscriberMetrics

	workManager workerpool.Manager
}

func (s *subscriber) Ref() string {
	return s.reference
}

func (s *subscriber) URI() string {
	return s.url
}

func (s *subscriber) Receive(ctx context.Context) (*pubsub.Message, error) {
	if s.subscription == nil {
		return nil, errors.New("only initialised subscriptions can pull messages")
	}

	s.state = SubscriberStateWaiting
	s.metrics.LastActivity.Store(time.Now().UnixNano())

	msg, err := s.subscription.Receive(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		s.state = SubscriberStateInError
		s.metrics.ErrorCount.Add(1)
		return nil, err
	}
	s.state = SubscriberStateProcessing
	s.metrics.ActiveMessages.Add(1)
	return msg, nil
}

func (s *subscriber) createSubscription(ctx context.Context) error {
	if s.subscription != nil {
		return nil
	}

	// Validate URL before attempting to open subscription
	if strings.TrimSpace(s.url) == "" {
		return fmt.Errorf("subscriber URL cannot be empty")
	}

	if !strings.HasPrefix(s.url, "http") {
		subs, err := pubsub.OpenSubscription(ctx, s.url)
		if err != nil {
			return fmt.Errorf("could not open topic subscription: %w", err)
		}
		s.subscription = subs
	}

	return nil
}

func (s *subscriber) Init(ctx context.Context) error {
	if s.isInit.Load() && s.subscription != nil {
		return nil
	}

	err := s.createSubscription(ctx)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(s.url, "http") {
		if s.handlers != nil {
			go s.listen(ctx)
		}
	}

	s.isInit.Store(true)
	return nil
}

func (s *subscriber) recreateSubscription(ctx context.Context) {
	log := util.Log(ctx).WithField("subscriber", s.reference)

	if !s.isInit.Load() {
		log.Error("only initialised subscriptions can be recreated")
	}

	log.Warn("recreating subscription")

	if s.subscription != nil {
		err := s.subscription.Shutdown(ctx)
		if err != nil {
			log.WithError(err).Error("could not recreate subscription, stopping listener")
			s.workManager.StopError(ctx, err)
		}
		s.subscription = nil
	}

	err := s.createSubscription(ctx)
	if err != nil {
		log.WithError(err).Error("could not recreate subscription, stopping listener")
		s.workManager.StopError(ctx, err)
	}
}

func (s *subscriber) Initiated() bool {
	return s.isInit.Load()
}

func (s *subscriber) State() SubscriberState {
	return s.state
}

func (s *subscriber) Metrics() SubscriberMetrics {
	return s.metrics
}

func (s *subscriber) IsIdle() bool {
	return s.metrics.IsIdle(s.state)
}

func (s *subscriber) Stop(ctx context.Context) error {
	// TODO: incooporate trace information in shutdown context
	var sctx context.Context
	var cancelFunc context.CancelFunc

	select {
	case <-ctx.Done():
		sctx = context.Background()
	default:
		sctx = ctx
	}

	sctx, cancelFunc = context.WithTimeout(sctx, time.Second*1)
	defer cancelFunc()

	s.isInit.Store(false)

	if s.subscription != nil {
		err := s.subscription.Shutdown(sctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *subscriber) processReceivedMessage(ctx context.Context, msg *pubsub.Message) error {
	job := workerpool.NewJob[any](func(jobCtx context.Context, _ workerpool.JobResultPipe[any]) error {
		var err error
		defer s.metrics.closeMessage(time.Now(), err)

		var metadata propagation.MapCarrier = msg.Metadata

		pCtx := security.SkipTenancyChecksFromMap(jobCtx, metadata)

		authClaim := security.ClaimsFromMap(metadata)
		if authClaim != nil {
			pCtx = authClaim.ClaimsToContext(pCtx)
		}

		pCtx = otel.GetTextMapPropagator().Extract(pCtx, metadata)

		languages := localization.FromMap(metadata)
		if len(languages) > 0 {
			pCtx = localization.ToContext(pCtx, languages)
		}

		for _, worker := range s.handlers {
			err = worker.Handle(pCtx, metadata, msg.Body)
			if err != nil {
				logger := util.Log(pCtx).
					WithField("name", s.reference).
					WithField("function", "processReceivedMessage").
					WithField("url", s.url)
				logger.WithError(err).Warn("could not handle message")
				msg.Nack()
				return err // Propagate handlers error to the job runner
			}
		}
		msg.Ack()
		return nil
	})

	submitErr := workerpool.SubmitJob[any](ctx, s.workManager, job)
	if submitErr != nil {
		msg.Nack()
		logger := util.Log(ctx).
			WithField("name", s.reference).
			WithField("function", "processReceivedMessage").
			WithField("url", s.url)
		logger.WithError(submitErr).Error("could not process message, failed to submit job")
		s.metrics.closeMessage(time.Now(), submitErr)
		return submitErr
	}

	return nil
}

func (s *subscriber) listen(ctx context.Context) {
	logger := util.Log(ctx).
		WithField("name", s.reference).
		WithField("function", "subscription").
		WithField("url", s.url)
	logger.Debug("starting to listen for messages")
	for {
		select {
		case <-ctx.Done():
			err := s.Stop(ctx)
			if err != nil {
				logger.WithError(err).Error("could not stop subscription")
				return
			}
			logger.Debug("exiting due to canceled context")
			return

		default:
			msg, err := s.Receive(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					// Context cancelled or deadline exceeded, loop again to check ctx.Done()
					continue
				}
				// Other errors from Receive are critical for the listener.
				logger.WithError(err).Error("could not pull message")

				// Recreate subscription
				s.recreateSubscription(ctx)
				continue
			}

			// Process the received message. Errors from processing (like job submission failure)
			// will be logged by processReceivedMessage. If it's a critical submission error,
			// it will be returned and will stop the whole application.
			if procErr := s.processReceivedMessage(ctx, msg); procErr != nil {
				// processReceivedMessage already logs details. This error is for critical failures.
				logger.WithError(procErr).Error("critical error processing message, stopping listener")
				s.SendStopError(ctx, procErr) // procErr
				return                        // Exit listen loop
			}
		}
	}
}

func (s *subscriber) SendStopError(ctx context.Context, err error) {
	if s.workManager != nil {
		s.workManager.StopError(ctx, err)
	}
}

func newSubscriber(
	workPool workerpool.Manager,
	reference string,
	queueURL string,
	handlers ...SubscribeWorker,
) Subscriber {
	return &subscriber{
		reference: reference,
		url:       queueURL,
		handlers:  handlers,
		metrics: &subscriberMetrics{
			ActiveMessages: &atomic.Int64{},
			LastActivity:   &atomic.Int64{},
			ProcessingTime: &atomic.Int64{},
			MessageCount:   &atomic.Int64{},
			ErrorCount:     &atomic.Int64{},
		},
		workManager: workPool,
	}
}
