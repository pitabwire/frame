package framequeue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"gocloud.dev/pubsub"
)

// publisher implements the Publisher interface
type publisher struct {
	reference string
	queueURL  string
	topic     *pubsub.Topic
	initiated bool
	mutex     sync.RWMutex
	logger    Logger
}

// NewPublisher creates a new publisher instance
func NewPublisher(reference string, queueURL string, logger Logger) Publisher {
	return &publisher{
		reference: reference,
		queueURL:  queueURL,
		logger:    logger,
	}
}

// Initiated returns whether the publisher has been initialized
func (p *publisher) Initiated() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.initiated
}

// Ref returns the publisher reference
func (p *publisher) Ref() string {
	return p.reference
}

// Init initializes the publisher by opening the topic
func (p *publisher) Init(ctx context.Context) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.initiated {
		return nil
	}

	topic, err := pubsub.OpenTopic(ctx, p.queueURL)
	if err != nil {
		if p.logger != nil {
			p.logger.WithError(err).WithField("reference", p.reference).WithField("queueURL", p.queueURL).Error("Failed to open topic")
		}
		return fmt.Errorf("failed to open topic for publisher %s: %w", p.reference, err)
	}

	p.topic = topic
	p.initiated = true

	if p.logger != nil {
		p.logger.WithField("reference", p.reference).WithField("queueURL", p.queueURL).Info("Publisher initialized successfully")
	}

	return nil
}

// Publish publishes a message to the topic
func (p *publisher) Publish(ctx context.Context, payload any, headers ...map[string]string) error {
	p.mutex.RLock()
	if !p.initiated {
		p.mutex.RUnlock()
		return fmt.Errorf("publisher %s is not initialized", p.reference)
	}
	topic := p.topic
	p.mutex.RUnlock()

	// Serialize payload
	body, err := json.Marshal(payload)
	if err != nil {
		if p.logger != nil {
			p.logger.WithError(err).WithField("reference", p.reference).Error("Failed to marshal payload")
		}
		return fmt.Errorf("failed to marshal payload for publisher %s: %w", p.reference, err)
	}

	// Prepare message
	message := &pubsub.Message{
		Body:     body,
		Metadata: make(map[string]string),
	}

	// Add headers to metadata
	for _, headerMap := range headers {
		for key, value := range headerMap {
			message.Metadata[key] = value
		}
	}

	// Send message
	err = topic.Send(ctx, message)
	if err != nil {
		if p.logger != nil {
			p.logger.WithError(err).WithField("reference", p.reference).Error("Failed to send message")
		}
		return fmt.Errorf("failed to send message for publisher %s: %w", p.reference, err)
	}

	if p.logger != nil {
		p.logger.WithField("reference", p.reference).WithField("payloadSize", len(body)).Debug("Message published successfully")
	}

	return nil
}

// Stop stops the publisher and closes the topic
func (p *publisher) Stop(ctx context.Context) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.initiated {
		return nil
	}

	var err error
	if p.topic != nil {
		err = p.topic.Shutdown(ctx)
		if err != nil && p.logger != nil {
			p.logger.WithError(err).WithField("reference", p.reference).Error("Error shutting down topic")
		}
		p.topic = nil
	}

	p.initiated = false

	if p.logger != nil {
		p.logger.WithField("reference", p.reference).Info("Publisher stopped")
	}

	return err
}
