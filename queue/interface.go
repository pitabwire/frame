package queue

import (
	"context"
	"time"

	"gocloud.dev/pubsub"
)

type SubscriberState int32

const (
	SubscriberStateWaiting SubscriberState = iota
	SubscriberStateProcessing
	SubscriberStateInError
)

type Manager interface {
	AddPublisher(ctx context.Context, reference string, queueURL string) error
	GetPublisher(reference string) (Publisher, error)
	DiscardPublisher(ctx context.Context, reference string) error

	AddSubscriber(
		ctx context.Context,
		reference string,
		queueURL string,
		handlers ...SubscribeWorker,
	) error
	DiscardSubscriber(ctx context.Context, reference string) error
	GetSubscriber(reference string) (Subscriber, error)

	Publish(ctx context.Context, reference string, payload any, headers ...map[string]string) error
	Init(ctx context.Context) error
	Close(ctx context.Context) error
}

// PushLookup is implemented by the concrete queue manager for HTTP push demux.
// Kept separate from Manager so application test doubles of Manager stay compatible.
type PushLookup interface {
	// HasPushSubscribers reports whether any registered subscriber is push mode.
	HasPushSubscribers() bool
	// LookupPush returns a push target by registration reference.
	LookupPush(ref string) (PushTarget, bool)
	GetSubscriber(reference string) (Subscriber, error)
}

// PushTarget is the narrow surface the HTTP multiplexing handler uses.
type PushTarget interface {
	Ref() string
	Mode() DeliveryMode
	ProcessPush(ctx context.Context, metadata map[string]string, body []byte) error
}

type Publisher interface {
	Initiated() bool
	Ref() string
	Init(ctx context.Context) error

	Publish(ctx context.Context, payload any, headers ...map[string]string) error
	Stop(ctx context.Context) error
	As(i any) bool
}

type Subscriber interface {
	Ref() string

	URI() string
	Initiated() bool
	State() SubscriberState
	Metrics() SubscriberMetrics
	IsIdle() bool
	// Mode reports pull vs push delivery.
	Mode() DeliveryMode

	Init(ctx context.Context) error
	Receive(ctx context.Context) (*pubsub.Message, error)
	Stop(ctx context.Context) error
	As(i any) bool

	// ProcessPush delivers one push message synchronously (push mode only).
	ProcessPush(ctx context.Context, metadata map[string]string, body []byte) error
}

type SubscribeWorker interface {
	Handle(ctx context.Context, metadata map[string]string, message []byte) error
}

type SubscriberMetrics interface {
	IsIdle(state SubscriberState) bool
	IdleTime(state SubscriberState) time.Duration
	AverageProcessingTime() time.Duration
}
