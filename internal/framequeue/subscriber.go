package framequeue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gocloud.dev/pubsub"
)

// subscriber implements the Subscriber interface
type subscriber struct {
	reference string
	queueURL  string
	handlers  []SubscribeWorker
	
	subscription *pubsub.Subscription
	initiated    bool
	state        SubscriberState
	metrics      *SubscriberMetrics
	
	mutex  sync.RWMutex
	logger Logger
}

// NewSubscriber creates a new subscriber instance
func NewSubscriber(reference string, queueURL string, logger Logger, handlers ...SubscribeWorker) Subscriber {
	return &subscriber{
		reference: reference,
		queueURL:  queueURL,
		handlers:  handlers,
		state:     SubscriberStateWaiting,
		metrics: &SubscriberMetrics{
			ActiveMessages: &atomic.Int64{},
			LastActivity:   &atomic.Int64{},
			ProcessingTime: &atomic.Int64{},
			MessageCount:   &atomic.Int64{},
			ErrorCount:     &atomic.Int64{},
		},
		logger: logger,
	}
}

// Ref returns the subscriber reference
func (s *subscriber) Ref() string {
	return s.reference
}

// URI returns the subscriber queue URL
func (s *subscriber) URI() string {
	return s.queueURL
}

// Initiated returns whether the subscriber has been initialized
func (s *subscriber) Initiated() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.initiated
}

// State returns the current subscriber state
func (s *subscriber) State() SubscriberState {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.state
}

// Metrics returns the subscriber metrics
func (s *subscriber) Metrics() *SubscriberMetrics {
	return s.metrics
}

// IsIdle returns true if the subscriber is idle
func (s *subscriber) IsIdle() bool {
	s.mutex.RLock()
	state := s.state
	s.mutex.RUnlock()
	
	return s.metrics.IsIdle(state)
}

// Init initializes the subscriber by opening the subscription
func (s *subscriber) Init(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.initiated {
		return nil
	}

	subscription, err := pubsub.OpenSubscription(ctx, s.queueURL)
	if err != nil {
		if s.logger != nil {
			s.logger.WithError(err).WithField("reference", s.reference).WithField("queueURL", s.queueURL).Error("Failed to open subscription")
		}
		return fmt.Errorf("failed to open subscription for subscriber %s: %w", s.reference, err)
	}

	s.subscription = subscription
	s.initiated = true
	s.state = SubscriberStateWaiting

	if s.logger != nil {
		s.logger.WithField("reference", s.reference).WithField("queueURL", s.queueURL).Info("Subscriber initialized successfully")
	}

	return nil
}

// Receive receives a message from the subscription
func (s *subscriber) Receive(ctx context.Context) (*pubsub.Message, error) {
	s.mutex.RLock()
	if !s.initiated {
		s.mutex.RUnlock()
		return nil, fmt.Errorf("subscriber %s is not initialized", s.reference)
	}
	subscription := s.subscription
	s.mutex.RUnlock()

	// Update state to processing
	s.mutex.Lock()
	s.state = SubscriberStateProcessing
	s.mutex.Unlock()

	// Track active message
	s.metrics.ActiveMessages.Add(1)
	s.metrics.LastActivity.Store(time.Now().UnixNano())

	startTime := time.Now()
	defer func() {
		// Update processing time and reset state
		processingDuration := time.Since(startTime)
		s.metrics.ProcessingTime.Add(processingDuration.Nanoseconds())
		s.metrics.MessageCount.Add(1)
		s.metrics.ActiveMessages.Add(-1)
		
		s.mutex.Lock()
		s.state = SubscriberStateWaiting
		s.mutex.Unlock()
	}()

	message, err := subscription.Receive(ctx)
	if err != nil {
		s.metrics.ErrorCount.Add(1)
		
		s.mutex.Lock()
		s.state = SubscriberStateInError
		s.mutex.Unlock()
		
		if s.logger != nil {
			s.logger.WithError(err).WithField("reference", s.reference).Error("Failed to receive message")
		}
		return nil, fmt.Errorf("failed to receive message for subscriber %s: %w", s.reference, err)
	}

	// Process message with handlers
	if len(s.handlers) > 0 {
		for _, handler := range s.handlers {
			if err := handler.Handle(ctx, message.Metadata, message.Body); err != nil {
				s.metrics.ErrorCount.Add(1)
				
				if s.logger != nil {
					s.logger.WithError(err).WithField("reference", s.reference).Error("Handler failed to process message")
				}
				
				// Nack the message on handler error
				message.Nack()
				return message, fmt.Errorf("handler failed to process message for subscriber %s: %w", s.reference, err)
			}
		}
		
		// Ack the message after successful processing
		message.Ack()
	}

	if s.logger != nil {
		s.logger.WithField("reference", s.reference).WithField("messageSize", len(message.Body)).Debug("Message received and processed successfully")
	}

	return message, nil
}

// Stop stops the subscriber and closes the subscription
func (s *subscriber) Stop(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.initiated {
		return nil
	}

	var err error
	if s.subscription != nil {
		err = s.subscription.Shutdown(ctx)
		if err != nil && s.logger != nil {
			s.logger.WithError(err).WithField("reference", s.reference).Error("Error shutting down subscription")
		}
		s.subscription = nil
	}

	s.initiated = false
	s.state = SubscriberStateWaiting

	if s.logger != nil {
		s.logger.WithField("reference", s.reference).Info("Subscriber stopped")
	}

	return err
}
