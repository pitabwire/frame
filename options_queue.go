package frame

import (
	"context"
	"errors"
	"fmt"
	"strings"

	_ "github.com/pitabwire/natspubsub" // required for NATS pubsub driver registration
	_ "gocloud.dev/pubsub/mempubsub"    // required for in-memory pubsub driver registration

	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/queue"
)

// WithRegisterPublisher Option to register publishing path referenced within the system.
func WithRegisterPublisher(reference string, queueURL string) Option {
	return func(_ context.Context, s *Service) {
		s.registerPlugin("queue")

		// Validate inputs and report via startup errors instead of panicking
		if strings.TrimSpace(reference) == "" {
			s.AddStartupError(errors.New("publisher reference cannot be empty"))
			return
		}
		if !data.DSN(queueURL).Valid() {
			s.AddStartupError(fmt.Errorf("publisher queueURL is invalid: %s", queueURL))
			return
		}

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
	return func(_ context.Context, s *Service) {
		// Validate inputs and report via startup errors instead of panicking
		if strings.TrimSpace(reference) == "" {
			s.AddStartupError(errors.New("subscriber reference cannot be empty"))
			return
		}
		if !data.DSN(queueURL).Valid() {
			s.AddStartupError(fmt.Errorf("subscriber queueURL is invalid: %s", queueURL))
			return
		}

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
