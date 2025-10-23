package frame

import (
	"context"

	_ "github.com/pitabwire/natspubsub" // required for NATS pubsub driver registration
	_ "gocloud.dev/pubsub/mempubsub"    // required for in-memory pubsub driver registration

	"github.com/pitabwire/frame/queue"
)

// WithRegisterPublisher Option to register publishing path referenced within the system.
func WithRegisterPublisher(reference string, queueURL string) Option {
	return func(_ context.Context, s *Service) {
		// Queue manager is initialized after options are applied,
		// so defer registration to pre-start phase
		// Publishers must be registered before subscribers (for mem:// driver)
		s.AddPublisherStartup(func(ctx context.Context, svc *Service) {
			err := svc.Queue(ctx).AddPublisher(ctx, reference, queueURL)
			if err != nil {
				svc.Log(ctx).WithError(err).
					WithField("publisher_ref", reference).
					WithField("publisher_url", queueURL).
					Error("Failed to register publisher")
				svc.AddStartupError(err)
			}
		})
	}
}

// WithRegisterSubscriber Option to register a new subscription handlers.
func WithRegisterSubscriber(reference string, queueURL string,
	handlers ...queue.SubscribeWorker) Option {
	return func(_ context.Context, s *Service) {
		// Queue manager is initialized after options are applied,
		// so defer registration to pre-start phase
		// Subscribers must be registered after publishers (for mem:// driver)
		s.AddSubscriberStartup(func(ctx context.Context, svc *Service) {
			err := svc.Queue(ctx).AddSubscriber(ctx, reference, queueURL, handlers...)
			if err != nil {
				svc.Log(ctx).WithError(err).
					WithField("subscriber_ref", reference).
					WithField("subscriber_url", queueURL).
					Error("Failed to register subscriber")
				svc.AddStartupError(err)
			}
		})
	}
}

func (s *Service) Queue(_ context.Context) queue.Manager {
	return s.queueManager
}
