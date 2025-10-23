package queue

import (
	"context"
	"fmt"
	"sync"

	"github.com/pitabwire/util"
)

type queue struct {
	stopMutex            sync.Mutex
	publishQueueMap      *sync.Map
	subscriptionQueueMap *sync.Map
}

func NewQueueManager(_ context.Context) Manager {
	q := &queue{
		publishQueueMap:      &sync.Map{},
		subscriptionQueueMap: &sync.Map{},
	}

	return q
}

func (s *queue) AddPublisher(ctx context.Context, reference string, queueURL string) error {
	pub, _ := s.GetPublisher(reference)
	if pub != nil {
		return nil
	}

	pub = newPublisher(reference, queueURL)
	err := pub.Init(ctx)
	if err != nil {
		return err
	}

	s.publishQueueMap.Store(reference, pub)
	return nil
}

func (s *queue) DiscardPublisher(ctx context.Context, reference string) error {
	var err error
	pub, _ := s.GetPublisher(reference)
	if pub != nil {
		err = pub.Stop(ctx)
	}

	s.publishQueueMap.Delete(reference)
	return err
}

func (s *queue) GetPublisher(reference string) (Publisher, error) {
	pub, ok := s.publishQueueMap.Load(reference)
	if !ok {
		return nil, fmt.Errorf("publisher %s not found", reference)
	}
	pVal, ok := pub.(*publisher)
	if !ok {
		return nil, fmt.Errorf("publisher %s is not of type *publisher", reference)
	}
	return pVal, nil
}

func (s *queue) AddSubscriber(
	ctx context.Context,
	reference string,
	queueURL string,
	handlers ...SubscribeWorker,
) error {
	subs0, _ := s.GetSubscriber(reference)
	if subs0 != nil {
		return nil
	}

	subs := newSubscriber(reference, queueURL, handlers...)
	err := s.initSubscriber(ctx, subs)
	if err != nil {
		return err
	}

	s.subscriptionQueueMap.Store(reference, subs)

	return nil
}

func (s *queue) DiscardSubscriber(ctx context.Context, reference string) error {
	var err error
	sub, _ := s.GetSubscriber(reference)
	if sub != nil {
		err = sub.Stop(ctx)
	}

	s.subscriptionQueueMap.Delete(reference)
	return err
}

func (s *queue) GetSubscriber(reference string) (Subscriber, error) {
	sub, ok := s.subscriptionQueueMap.Load(reference)
	if !ok {
		return nil, fmt.Errorf("subscriber %s not found", reference)
	}
	sVal, ok := sub.(*subscriber)
	if !ok {
		return nil, fmt.Errorf("subscriber %s is not of type *subscriber", reference)
	}
	return sVal, nil
}

// Publish ByIsQueue method to write a new message into the queue pre initialized with the supplied reference.
func (s *queue) Publish(ctx context.Context, reference string, payload any, headers ...map[string]string) error {
	pub, err := s.GetPublisher(reference)
	if err != nil {
		return err
	}

	return pub.Publish(ctx, payload, headers...)
}

func (s *queue) initSubscriber(ctx context.Context, sub Subscriber) error {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	return sub.Init(ctx)
}

// initializeRegisteredPublishers iterates over and initializes all registered publishers.
func (s *queue) initializeRegisteredPublishers(ctx context.Context) error {
	var initErrors []error
	s.publishQueueMap.Range(func(key, value any) bool {
		pub, ok := value.(*publisher)
		if !ok {
			util.Log(ctx).WithField("key", key).
				WithField("actual_type", fmt.Sprintf("%T", value)).
				Warn("Item in publishQueueMap is not of type *publisher, skipping initialization.")
			return true // continue to next item
		}
		if err := pub.Init(ctx); err != nil {
			util.Log(ctx).WithError(err).
				WithField("publisher_ref", pub.Ref()).
				WithField("publisher_url", pub.url).
				Error("Failed to initialize publisher")
			initErrors = append(initErrors, fmt.Errorf("publisher %s: %w", pub.Ref(), err))
		}
		return true
	})

	if len(initErrors) > 0 {
		// Consider how to aggregate multiple errors. For now, return the first one.
		// Or use a multierror package if available/preferred.
		return fmt.Errorf("failed to initialize one or more publishers: %w", initErrors[0])
	}
	return nil
}

// initializeRegisteredSubscribers iterates over and initializes all registered subscribers.
func (s *queue) initializeRegisteredSubscribers(ctx context.Context) error {
	var initErrors []error
	s.subscriptionQueueMap.Range(func(key, value any) bool {
		sub, ok := value.(Subscriber)
		if !ok {
			util.Log(ctx).WithField("key", key).
				WithField("actual_type", fmt.Sprintf("%T", value)).
				Warn("Item in subscriptionQueueMap is not of type *subscriber, skipping initialization.")
			return true // continue to next item
		}
		if err := s.initSubscriber(ctx, sub); err != nil {
			util.Log(ctx).WithError(err).
				WithField("subscriber_ref", sub.Ref()).
				WithField("subscriber_url", sub.URI()).
				Error("Failed to initialize subscriber")
			initErrors = append(initErrors, fmt.Errorf("subscriber %s: %w", sub.Ref(), err))
		}
		return true
	})

	if len(initErrors) > 0 {
		return fmt.Errorf("failed to initialize one or more subscribers: %w", initErrors[0])
	}
	return nil
}

func (s *queue) Init(ctx context.Context) error {
	if s == nil {
		util.Log(ctx).Debug(
			"No generic queue backend configured (s.queue is nil), skipping further pub/sub initialization.",
		)
		return nil
	}

	if err := s.initializeRegisteredPublishers(ctx); err != nil {
		// Errors logged by helper
		return fmt.Errorf("failed during publisher initialization: %w", err)
	}

	if err := s.initializeRegisteredSubscribers(ctx); err != nil {
		// Errors logged by helper
		return fmt.Errorf("failed during subscriber initialization: %w", err)
	}

	util.Log(ctx).Info("Pub/Sub system initialized successfully.")
	return nil
}
