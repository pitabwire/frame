package frame

import (
	"context"
	"errors"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/events"
)

// WithRegisterEvents registers events for the service. All events are unique and shouldn't share a name otherwise the last one registered will take precedence.
func WithRegisterEvents(evt ...events.EventI) Option {
	return func(_ context.Context, s *Service) {
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
func (s *Service) setupEventsQueue(ctx context.Context) error {
	cfg, ok := s.Config().(config.ConfigurationEvents)
	if !ok {
		errMsg := "configuration object does not implement ConfigurationEvents, cannot setup events queue"
		s.Log(ctx).Error(errMsg)
		return errors.New(errMsg)
	}

	s.eventsManager = events.NewManager(ctx, s.QueueManager(), cfg)

	eventsQueueSubscriberOpt := WithRegisterSubscriber(
		cfg.GetEventsQueueName(),
		cfg.GetEventsQueueURL(),
		s.eventsManager.Handler(),
	)
	eventsQueueSubscriberOpt(ctx, s) // This registers the subscriber

	eventsQueuePublisherOpt := WithRegisterPublisher(cfg.GetEventsQueueName(), cfg.GetEventsQueueURL())
	eventsQueuePublisherOpt(ctx, s) // This registers the publisher

	// Note: Actual initialization of this specific subscriber and publisher
	// will happen in initializeRegisteredPublishers and initializeRegisteredSubscribers.
	return nil
}
