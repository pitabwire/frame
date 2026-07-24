package frame

import (
	"context"
	"errors"

	"github.com/pitabwire/frame/v2/config"
	"github.com/pitabwire/frame/v2/events"
)

// WithRegisterEvents registers events for the service. All events are unique and shouldn't share a name otherwise the last one registered will take precedence.
func WithRegisterEvents(evt ...events.EventI) Option {
	return func(_ context.Context, s *Service) {
		s.registerPlugin("events")

		// Events manager is initialized in setupEventsQueue after options are applied
		// so defer event registration to pre-start phase
		s.AddPreStartMethod(func(_ context.Context, svc *Service) {
			for _, event := range evt {
				svc.eventsManager.Add(event)
			}
		})
	}
}

func (s *Service) EventsManager() events.Manager {
	return s.eventsManager
}

// setupEventsQueue sets up the default events queue publisher and subscriber
// if an event registry is configured for the service.
//
// Publish and subscribe URLs may differ (GCP Pub/Sub push pattern):
//   - EVENTS_QUEUE_PUBLISH_URL=gcppubsub://project/topic
//   - EVENTS_QUEUE_SUBSCRIBE_URL=push://ref  (GCP push → POST /_frame/queue/{ref})
//
// When the override envs are empty, both sides use EVENTS_QUEUE_URL.
func (s *Service) setupEventsQueue(ctx context.Context) error {
	cfg, ok := s.Config().(config.ConfigurationEvents)
	if !ok {
		errMsg := "configuration object does not implement ConfigurationEvents, cannot setup events queue"
		s.Log(ctx).Error(errMsg)
		return errors.New(errMsg)
	}

	s.eventsManager = events.NewManager(ctx, s.QueueManager(), cfg)

	ref := cfg.GetEventsQueueName()
	pubURL := cfg.GetEventsQueuePublishURL()
	subURL := cfg.GetEventsQueueSubscribeURL()

	eventsQueueSubscriberOpt := WithRegisterSubscriber(
		ref,
		subURL,
		s.eventsManager.Handler(),
	)
	eventsQueueSubscriberOpt(ctx, s)

	eventsQueuePublisherOpt := WithRegisterPublisher(ref, pubURL)
	eventsQueuePublisherOpt(ctx, s)

	return nil
}
