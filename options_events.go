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

// Emit a simple method used to deploy.
func (s *Service) Emit(ctx context.Context, name string, payload any) error {
	cfg, ok := s.Config().(config.ConfigurationEvents)
	if !ok {
		s.Log(ctx).Warn("configuration object not of type : ConfigurationDefault")
		return errors.New("could not cast cfg to ConfigurationEvents")
	}

	// ByIsQueue event message for further processing
	err := s.Queue(ctx).Publish(ctx, cfg.GetEventsQueueName(), payload, map[string]string{events.EventHeaderName: name})
	if err != nil {
		s.Log(ctx).WithError(err).WithField("name", name).Error("Could not emit event")
		return err
	}

	return nil
}

// setupEventsQueue sets up the default events queue publisher and subscriber
// if an event registry is configured for the service.
func (s *Service) setupEventsQueue(ctx context.Context) error {
	s.eventsManager = events.NewManager(ctx)

	cfg, ok := s.Config().(config.ConfigurationEvents)
	if !ok {
		errMsg := "configuration object does not implement ConfigurationEvents, cannot setup events queue"
		s.Log(ctx).Error(errMsg)
		return errors.New(errMsg)
	}

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
