package framequeue

import (
	"context"
	"fmt"
	"sync"
)

// queueManager implements the QueueManager interface
type queueManager struct {
	publishers  map[string]Publisher
	subscribers map[string]Subscriber
	
	config Config
	logger Logger
	
	mutex sync.RWMutex
}

// NewQueueManager creates a new queue manager instance
func NewQueueManager(config Config, logger Logger) QueueManager {
	return &queueManager{
		publishers:  make(map[string]Publisher),
		subscribers: make(map[string]Subscriber),
		config:      config,
		logger:      logger,
	}
}

// AddPublisher adds a new publisher with the given reference and queue URL
func (qm *queueManager) AddPublisher(ctx context.Context, reference string, queueURL string) error {
	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	if _, exists := qm.publishers[reference]; exists {
		return fmt.Errorf("publisher with reference %s already exists", reference)
	}

	publisher := NewPublisher(reference, queueURL, qm.logger)
	if err := publisher.Init(ctx); err != nil {
		return fmt.Errorf("failed to initialize publisher %s: %w", reference, err)
	}

	qm.publishers[reference] = publisher

	if qm.logger != nil {
		qm.logger.WithField("reference", reference).WithField("queueURL", queueURL).Info("Publisher added successfully")
	}

	return nil
}

// DiscardPublisher removes and stops a publisher
func (qm *queueManager) DiscardPublisher(ctx context.Context, reference string) error {
	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	publisher, exists := qm.publishers[reference]
	if !exists {
		return fmt.Errorf("publisher with reference %s does not exist", reference)
	}

	if err := publisher.Stop(ctx); err != nil {
		if qm.logger != nil {
			qm.logger.WithError(err).WithField("reference", reference).Error("Error stopping publisher")
		}
		// Continue with removal even if stop fails
	}

	delete(qm.publishers, reference)

	if qm.logger != nil {
		qm.logger.WithField("reference", reference).Info("Publisher discarded successfully")
	}

	return nil
}

// GetPublisher retrieves a publisher by reference
func (qm *queueManager) GetPublisher(reference string) (Publisher, error) {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	publisher, exists := qm.publishers[reference]
	if !exists {
		return nil, fmt.Errorf("publisher with reference %s does not exist", reference)
	}

	return publisher, nil
}

// Publish publishes a message using the specified publisher
func (qm *queueManager) Publish(ctx context.Context, reference string, payload any, headers ...map[string]string) error {
	publisher, err := qm.GetPublisher(reference)
	if err != nil {
		return err
	}

	return publisher.Publish(ctx, payload, headers...)
}

// AddSubscriber adds a new subscriber with the given reference, queue URL, and handlers
func (qm *queueManager) AddSubscriber(ctx context.Context, reference string, queueURL string, handlers ...SubscribeWorker) error {
	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	if _, exists := qm.subscribers[reference]; exists {
		return fmt.Errorf("subscriber with reference %s already exists", reference)
	}

	subscriber := NewSubscriber(reference, queueURL, qm.logger, handlers...)
	if err := subscriber.Init(ctx); err != nil {
		return fmt.Errorf("failed to initialize subscriber %s: %w", reference, err)
	}

	qm.subscribers[reference] = subscriber

	if qm.logger != nil {
		qm.logger.WithField("reference", reference).WithField("queueURL", queueURL).WithField("handlerCount", len(handlers)).Info("Subscriber added successfully")
	}

	return nil
}

// DiscardSubscriber removes and stops a subscriber
func (qm *queueManager) DiscardSubscriber(ctx context.Context, reference string) error {
	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	subscriber, exists := qm.subscribers[reference]
	if !exists {
		return fmt.Errorf("subscriber with reference %s does not exist", reference)
	}

	if err := subscriber.Stop(ctx); err != nil {
		if qm.logger != nil {
			qm.logger.WithError(err).WithField("reference", reference).Error("Error stopping subscriber")
		}
		// Continue with removal even if stop fails
	}

	delete(qm.subscribers, reference)

	if qm.logger != nil {
		qm.logger.WithField("reference", reference).Info("Subscriber discarded successfully")
	}

	return nil
}

// GetSubscriber retrieves a subscriber by reference
func (qm *queueManager) GetSubscriber(reference string) (Subscriber, error) {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	subscriber, exists := qm.subscribers[reference]
	if !exists {
		return nil, fmt.Errorf("subscriber with reference %s does not exist", reference)
	}

	return subscriber, nil
}

// InitializePubSub initializes the pub/sub system
func (qm *queueManager) InitializePubSub(ctx context.Context) error {
	if qm.config == nil {
		if qm.logger != nil {
			qm.logger.Warn("No configuration provided for pub/sub initialization")
		}
		return nil
	}

	// Initialize events queue if configured
	eventsQueueName := qm.config.GetEventsQueueName()
	eventsQueueURL := qm.config.GetEventsQueueURL()

	if eventsQueueName != "" && eventsQueueURL != "" {
		// Add events publisher
		if err := qm.AddPublisher(ctx, eventsQueueName+"_publisher", eventsQueueURL); err != nil {
			return fmt.Errorf("failed to initialize events publisher: %w", err)
		}

		// Add events subscriber
		if err := qm.AddSubscriber(ctx, eventsQueueName+"_subscriber", eventsQueueURL); err != nil {
			return fmt.Errorf("failed to initialize events subscriber: %w", err)
		}

		if qm.logger != nil {
			qm.logger.WithField("eventsQueueName", eventsQueueName).WithField("eventsQueueURL", eventsQueueURL).Info("Events queue initialized successfully")
		}
	}

	if qm.logger != nil {
		qm.logger.Info("Pub/sub system initialized successfully")
	}

	return nil
}
