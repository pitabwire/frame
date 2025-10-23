package frame

import (
	"context"

	_ "github.com/pitabwire/natspubsub" // required for NATS pubsub driver registration
	_ "gocloud.dev/pubsub/mempubsub"    // required for in-memory pubsub driver registration

	"github.com/pitabwire/frame/queue"
)

// WithRegisterPublisher Option to register publishing path referenced within the system.
func WithRegisterPublisher(reference string, queueURL string) Option {
	return func(ctx context.Context, s *Service) {
		err := s.Queue(ctx).AddPublisher(ctx, reference, queueURL)
		if err != nil {
			s.Log(ctx).WithError(err).
				WithField("publisher_ref", reference).
				WithField("publisher_url", queueURL).
				Error("Failed to register publisher")
		}
	}
}

// WithRegisterSubscriber Option to register a new subscription handlers.
func WithRegisterSubscriber(reference string, queueURL string,
	handlers ...queue.SubscribeWorker) Option {
	return func(ctx context.Context, s *Service) {
		err := s.Queue(ctx).AddSubscriber(ctx, reference, queueURL, handlers...)
		if err != nil {
			s.Log(ctx).WithError(err).
				WithField("subscriber_ref", reference).
				WithField("subscriber_url", queueURL).
				Error("Failed to register subscriber")
		}
	}
}

func (s *Service) Queue(_ context.Context) queue.Manager {
	return s.queueManager
}
