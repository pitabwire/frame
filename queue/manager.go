package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/workerpool"
)

type queueManager struct {
	stopMutex            sync.Mutex
	publishQueueMap      *sync.Map
	subscriptionQueueMap *sync.Map
	initialized          bool
	initMutex            sync.Mutex

	workPool workerpool.Manager
}

func NewQueueManager(_ context.Context, workPool workerpool.Manager) Manager {
	q := &queueManager{
		publishQueueMap:      &sync.Map{},
		subscriptionQueueMap: &sync.Map{},

		workPool: workPool,
	}

	return q
}

func (s *queueManager) AddPublisher(ctx context.Context, reference string, queueURL string) error {
	// Validate inputs before proceeding
	if strings.TrimSpace(reference) == "" {
		return errors.New("publisher reference cannot be empty")
	}
	if !data.DSN(queueURL).Valid() {
		return errors.New("publisher queueURL cannot be empty")
	}

	pub, _ := s.GetPublisher(reference)
	if pub != nil {
		return nil
	}

	pub = newPublisher(reference, queueURL)

	// Only initialize immediately if queueManager manager has already been initialized
	s.initMutex.Lock()
	alreadyInitialized := s.initialized
	s.initMutex.Unlock()

	if alreadyInitialized {
		err := pub.Init(ctx)
		if err != nil {
			return err
		}
	}

	s.publishQueueMap.Store(reference, pub)
	return nil
}

func (s *queueManager) DiscardPublisher(ctx context.Context, reference string) error {
	var err error
	pub, _ := s.GetPublisher(reference)
	if pub != nil {
		err = pub.Stop(ctx)
	}

	s.publishQueueMap.Delete(reference)
	return err
}

func (s *queueManager) GetPublisher(reference string) (Publisher, error) {
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

func (s *queueManager) AddSubscriber(
	ctx context.Context,
	reference string,
	queueURL string,
	handlers ...SubscribeWorker,
) error {
	// Validate inputs before proceeding
	if strings.TrimSpace(reference) == "" {
		return errors.New("subscriber reference cannot be empty")
	}
	if !data.DSN(queueURL).Valid() {
		return errors.New("subscriber queueURL cannot be empty")
	}

	subs0, _ := s.GetSubscriber(reference)
	if subs0 != nil {
		return nil
	}

	subs := newSubscriber(s.workPool, reference, queueURL, handlers...)

	// Only initialize immediately if queueManager manager has already been initialized
	s.initMutex.Lock()
	alreadyInitialized := s.initialized
	s.initMutex.Unlock()

	if alreadyInitialized {
		err := s.initSubscriber(ctx, subs)
		if err != nil {
			return err
		}
	}

	s.subscriptionQueueMap.Store(reference, subs)

	return nil
}

func (s *queueManager) DiscardSubscriber(ctx context.Context, reference string) error {
	var err error
	sub, _ := s.GetSubscriber(reference)
	if sub != nil {
		err = sub.Stop(ctx)
	}

	s.subscriptionQueueMap.Delete(reference)
	return err
}

func (s *queueManager) GetSubscriber(reference string) (Subscriber, error) {
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

// Publish ByIsQueue method to write a new message into the queueManager pre initialized with the supplied reference.
func (s *queueManager) Publish(ctx context.Context, reference string, payload any, headers ...map[string]string) error {
	pub, err := s.GetPublisher(reference)
	if err != nil {
		return err
	}

	return pub.Publish(ctx, payload, headers...)
}

func (s *queueManager) initSubscriber(ctx context.Context, sub Subscriber) error {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	return sub.Init(ctx)
}

// initializeRegisteredPublishers iterates over and initializes all registered publishers.
func (s *queueManager) initializeRegisteredPublishers(ctx context.Context) error {
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
func (s *queueManager) initializeRegisteredSubscribers(ctx context.Context) error {
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

func (s *queueManager) Init(ctx context.Context) error {
	if s == nil {
		util.Log(ctx).Debug(
			"No generic queueManager backend configured (s.queueManager is nil), skipping further pub/sub initialization.",
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

	// Mark queueManager as initialized
	s.initMutex.Lock()
	s.initialized = true
	s.initMutex.Unlock()

	util.Log(ctx).Info("Pub/Sub system initialized successfully.")
	return nil
}

func (s *queueManager) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}

	var closeErr error

	s.publishQueueMap.Range(func(key, value any) bool {
		pub, ok := value.(*publisher)
		if !ok {
			s.publishQueueMap.Delete(key)
			return true
		}

		if err := pub.Stop(ctx); err != nil && closeErr == nil {
			closeErr = err
		}

		s.publishQueueMap.Delete(key)
		return true
	})

	s.subscriptionQueueMap.Range(func(key, value any) bool {
		sub, ok := value.(*subscriber)
		if !ok {
			s.subscriptionQueueMap.Delete(key)
			return true
		}

		if err := sub.Stop(ctx); err != nil && closeErr == nil {
			closeErr = err
		}

		s.subscriptionQueueMap.Delete(key)
		return true
	})

	s.initMutex.Lock()
	s.initialized = false
	s.initMutex.Unlock()

	return closeErr
}
