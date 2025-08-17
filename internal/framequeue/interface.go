package framequeue

import (
	"context"
	"sync/atomic"
	"time"

	"gocloud.dev/pubsub"
)

// QueueManager defines the contract for queue management functionality
type QueueManager interface {
	// Publisher management
	AddPublisher(ctx context.Context, reference string, queueURL string) error
	DiscardPublisher(ctx context.Context, reference string) error
	GetPublisher(reference string) (Publisher, error)
	Publish(ctx context.Context, reference string, payload any, headers ...map[string]string) error

	// Subscriber management
	AddSubscriber(ctx context.Context, reference string, queueURL string, handlers ...SubscribeWorker) error
	DiscardSubscriber(ctx context.Context, reference string) error
	GetSubscriber(reference string) (Subscriber, error)

	// Initialization
	InitializePubSub(ctx context.Context) error
}

// Publisher defines the interface for message publishing
type Publisher interface {
	Initiated() bool
	Ref() string
	Init(ctx context.Context) error
	Publish(ctx context.Context, payload any, headers ...map[string]string) error
	Stop(ctx context.Context) error
}

// Subscriber defines the interface for message subscription
type Subscriber interface {
	Ref() string
	URI() string
	Initiated() bool
	State() SubscriberState
	Metrics() *SubscriberMetrics
	IsIdle() bool
	Init(ctx context.Context) error
	Receive(ctx context.Context) (*pubsub.Message, error)
	Stop(ctx context.Context) error
}

// SubscribeWorker defines the interface for message handlers
type SubscribeWorker interface {
	Handle(ctx context.Context, metadata map[string]string, message []byte) error
}

// SubscriberState represents the current state of a subscriber
type SubscriberState int

const (
	SubscriberStateWaiting SubscriberState = iota
	SubscriberStateProcessing
	SubscriberStateInError
)

// SubscriberMetrics tracks operational metrics for a subscriber
type SubscriberMetrics struct {
	ActiveMessages *atomic.Int64 // Currently active messages being processed
	LastActivity   *atomic.Int64 // Last activity timestamp in UnixNano
	ProcessingTime *atomic.Int64 // Total processing time in nanoseconds
	MessageCount   *atomic.Int64 // Total messages processed
	ErrorCount     *atomic.Int64 // Total number of errors encountered
}

// IsIdle returns true if subscriber is idle and in waiting state
func (m *SubscriberMetrics) IsIdle(state SubscriberState) bool {
	return state == SubscriberStateWaiting && m.ActiveMessages.Load() <= 0
}

// IdleTime returns the duration since last activity if the subscriber is idle
func (m *SubscriberMetrics) IdleTime(state SubscriberState) time.Duration {
	if !m.IsIdle(state) {
		return 0
	}

	lastActivity := m.LastActivity.Load()
	if lastActivity == 0 {
		return 0
	}

	return time.Since(time.Unix(0, lastActivity))
}

// AverageProcessingTime returns the average time spent processing messages
func (m *SubscriberMetrics) AverageProcessingTime() time.Duration {
	count := m.MessageCount.Load()
	if count == 0 {
		return 0
	}

	return time.Duration(m.ProcessingTime.Load() / count)
}

// Config defines the configuration interface for queue functionality
type Config interface {
	// GetEventsQueueName returns the events queue name
	GetEventsQueueName() string
	
	// GetEventsQueueURL returns the events queue URL
	GetEventsQueueURL() string
}

// WorkerPool defines the interface for job submission
type WorkerPool interface {
	// SubmitJob submits a job to the worker pool
	SubmitJob(ctx context.Context, job Job) error
}

// Job defines the interface for background jobs
type Job interface {
	Execute(ctx context.Context) error
}

// EventRegistry defines the interface for event management
type EventRegistry interface {
	// GetEventHandlers returns handlers for a specific event type
	GetEventHandlers(eventType string) []EventHandler
	
	// HasEvents returns true if any events are registered
	HasEvents() bool
}

// EventHandler defines the interface for event handlers
type EventHandler interface {
	Handle(ctx context.Context, event Event) error
}

// Event defines the interface for events
type Event interface {
	Type() string
	Data() interface{}
}

// Logger defines the logging interface needed by the queue module
type Logger interface {
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
}

// ClaimsProvider defines the interface for authentication claims
type ClaimsProvider interface {
	AsMetadata() map[string]string
}

// LanguageProvider defines the interface for language/localization
type LanguageProvider interface {
	ToMap(metadata map[string]string, languages []string) map[string]string
}
