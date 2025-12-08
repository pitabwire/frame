package frame

import (
	"context"
	"strings"

	"github.com/pitabwire/frame/data"
	_ "github.com/pitabwire/natspubsub" // required for NATS pubsub driver registration
	_ "gocloud.dev/pubsub/mempubsub"    // required for in-memory pubsub driver registration

	"github.com/pitabwire/frame/queue"
)

// WithRegisterPublisher Option to register publishing path referenced within the system.
func WithRegisterPublisher(reference string, queueURL string) Option {
	// Validate inputs immediately - fail fast
	if strings.TrimSpace(reference) == "" {
		panic("publisher reference cannot be empty")
	}
	if !data.DSN(queueURL).Valid() {
		panic("publisher queueURL cannot be invalid")
	}

	return func(_ context.Context, s *Service) {
		// QueueManager manager is initialized after options are applied,
		// so defer registration to pre-start phase
		// Publishers must be registered before subscribers (for mem:// driver)
		s.AddPublisherStartup(func(ctx context.Context, svc *Service) {
			err := svc.QueueManager().AddPublisher(ctx, reference, queueURL)
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
	// Validate inputs immediately - fail fast
	if strings.TrimSpace(reference) == "" {
		panic("subscriber reference cannot be empty")
	}
	if !data.DSN(queueURL).Valid() {
		panic("subscriber queueURL cannot be invalid")
	}

	return func(_ context.Context, s *Service) {
		// QueueManager manager is initialized after options are applied,
		// so defer registration to pre-start phase
		// Subscribers must be registered after publishers (for mem:// driver)
		s.AddSubscriberStartup(func(ctx context.Context, svc *Service) {
			err := svc.QueueManager().AddSubscriber(ctx, reference, queueURL, handlers...)
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

func (s *Service) QueueManager() queue.Manager {
	return s.queueManager
}
