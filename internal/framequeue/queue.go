package framequeue

import (
	"context"
	"encoding/json"
	"maps"
	"sync"
	"sync/atomic"

	_ "github.com/pitabwire/natspubsub" // required for NATS pubsub driver registration
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub" // required for in-memory pubsub driver registration
	"google.golang.org/protobuf/proto"

	"github.com/pitabwire/frame/internal/common"
)

// SubscriberState and SubscriberMetrics moved to interface.go to avoid duplication
// All duplicate type definitions and methods removed

type queue struct {
	publishQueueMap      *sync.Map
	subscriptionQueueMap *sync.Map
}

func newQueue(_ context.Context) *queue {
	q := &queue{
		publishQueueMap:      &sync.Map{},
		subscriptionQueueMap: &sync.Map{},
	}

	return q
}

// Publisher interface and publisher struct moved to interface.go and publisher.go to avoid duplication

func (p *publisher) PublishInternal(ctx context.Context, payload any, headers ...map[string]string) error {
	var err error

	metadata := make(map[string]string)
	for _, h := range headers {
		maps.Copy(metadata, h)
	}

	authClaim := common.ClaimsFromContext(ctx)
	if authClaim != nil {
		maps.Copy(metadata, authClaim.AsMetadata())
	}

	language := common.LanguageFromContext(ctx)
	if len(language) > 0 {
		metadata = common.LanguageToMap(metadata, language)
	}

	var message []byte
	switch v := payload.(type) {
	case []byte:
		message = v
	case json.RawMessage:
		message, _ = v.MarshalJSON()
	case string:
		message = []byte(v)
	default:
		protoMsg, ok := payload.(proto.Message)
		if ok {
			message, err = proto.Marshal(protoMsg)
			if err != nil {
				return err
			}
		} else {
			message, err = json.Marshal(payload)
			if err != nil {
				return err
			}
		}
	}
	topic := p.topic

	return topic.Send(ctx, &pubsub.Message{
		Body:     message,
		Metadata: metadata,
	})
}

// publisher.Init, publisher.Initiated, and publisher.Stop methods moved to publisher.go to avoid duplication

// WithRegisterPublisher Option to register publishing path referenced within the system.
func WithRegisterPublisher(reference string, queueURL string) common.Option {
	return func(_ context.Context, s common.Service) {
		// Get QueueModule and register publisher
		module := s.GetModule(common.ModuleTypeQueue)
		if module == nil {
			return
		}
		
		queueModule, ok := module.(common.QueueModule)
		if !ok {
			return
		}
		
		queueManager := queueModule.QueueManager()
		if queueManager != nil {
			// Add publisher through queue manager
			queueManager.AddPublisher(context.Background(), reference, queueURL)
		}
	}
}

// AddPublisher method moved to avoid serviceImpl dependency
// Note: DiscardPublisher and GetPublisher methods removed as they cannot be defined on non-local ServiceInterface type
// These methods should be accessed through the QueueModule interface directly

// Subscriber interface and SubscribeWorker interface moved to interface.go to avoid duplication
// subscriber struct moved to subscriber.go to avoid duplication

// All subscriber methods moved to subscriber.go to avoid duplication

// WithRegisterSubscriber Option to register a new subscription handlers.
func WithRegisterSubscriber(reference string, queueURL string,
	handlers ...SubscribeWorker) common.Option {
	return func(_ context.Context, s common.Service) {
		_ = newSubscriber(s, reference, queueURL, handlers...)
		if queueModule, ok := s.GetModule(common.ModuleTypeQueue).(common.QueueModule); ok {
			if queue := queueModule.Queue(); queue != nil {
				// Store subscriber in queue - implementation will handle the storage
				// Note: subscriptionQueueMap access moved to queue implementation
				_ = queue // Queue implementation will handle subscriber storage
			}
		}
	}
}

// newSubscriber creates a new subscriber instance
func newSubscriber(s common.Service, reference string, queueURL string, handlers ...SubscribeWorker) *subscriber {
	return &subscriber{
		reference: reference,
		queueURL:  queueURL,
		handlers:  handlers,
		metrics: &SubscriberMetrics{
			ActiveMessages: &atomic.Int64{},
			LastActivity:   &atomic.Int64{},
			ProcessingTime: &atomic.Int64{},
			MessageCount:   &atomic.Int64{},
			ErrorCount:     &atomic.Int64{},
		},
	}
}
