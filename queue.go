package frame

import (
	"context"
	"encoding/json"
	"fmt"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	"log"
	"strings"
)

type queue struct {
	publishQueueMap      map[string]*publisher
	subscriptionQueueMap map[string]*subscriber
}

func (q queue) getPublisherByReference(reference string) (*publisher, error) {
	p := q.publishQueueMap[reference]
	if p == nil {
		return nil, fmt.Errorf("reference does not exist")
	}

	if !p.isInit {
		return nil, fmt.Errorf("getPublisherByReference -- can't publish on uninitialized queue %v ", reference)
	}

	return p, nil
}

func newQueue() (*queue, error) {

	q := &queue{
		publishQueueMap:      make(map[string]*publisher),
		subscriptionQueueMap: make(map[string]*subscriber),
	}

	return q, nil
}

type publisher struct {
	reference string
	url       string
	pubTopic  *pubsub.Topic
	isInit    bool
}

type SubscribeWorker interface {
	Handle(ctx context.Context, message []byte) error
}

type subscriber struct {
	url          string
	concurrency  int
	handler      SubscribeWorker
	subscription *pubsub.Subscription
	isInit       bool
}

// RegisterPublisher Option to register publishing path referenced within the system
func RegisterPublisher(reference string, queueURL string) Option {
	return func(s *Service) {
		s.queue.publishQueueMap[reference] = &publisher{
			reference: reference,
			url:       queueURL,
			isInit:    false,
		}
	}
}

// RegisterSubscriber Option to register a new subscription handler
func RegisterSubscriber(reference string, queueUrl string, concurrency int,
	handler SubscribeWorker) Option {
	return func(s *Service) {
		s.queue.subscriptionQueueMap[reference] = &subscriber{
			url:         queueUrl,
			concurrency: concurrency,
			handler:     handler,
		}
	}
}

func (s *Service) SubscriptionIsInitiated(path string) bool {
	return s.queue.subscriptionQueueMap[path].isInit
}

// Publish Queue method to write a new message into the queue pre initialized with the supplied reference
func (s *Service) Publish(ctx context.Context, reference string, payload interface{}) error {
	var metadata map[string]string

	authClaim := ClaimsFromContext(ctx)
	if authClaim != nil {
		metadata = authClaim.AsMetadata()
	} else {
		metadata = make(map[string]string)
	}

	pub, err := s.queue.getPublisherByReference(reference)
	if err != nil {
		if err.Error() != "reference does not exist" {
			return err
		}

		pub = &publisher{
			reference: reference,
			url:       reference,
			pubTopic:  nil,
			isInit:    false,
		}
		err = s.initPublisher(ctx, pub)
		if err != nil {
			return err
		}
		s.queue.publishQueueMap[reference] = pub
	}
	var message []byte
	msg, ok := payload.([]byte)
	if !ok {
		msg, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		message = msg
	} else {
		message = msg
	}

	return pub.pubTopic.Send(ctx, &pubsub.Message{
		Body:     message,
		Metadata: metadata,
	})
}

func (s *Service) initPublisher(ctx context.Context, publisher *publisher) error {
	topic, err := pubsub.OpenTopic(ctx, publisher.url)
	if err != nil {
		return err
	}

	s.AddCleanupMethod(func() {
		err := topic.Shutdown(ctx)
		if err != nil {
			s.L().WithError(err).WithField("reference", publisher.reference).Info("publish topic could not be closed")
		}
	})

	publisher.pubTopic = topic
	publisher.isInit = true
	return nil
}

func (s *Service) initSubscriber(ctx context.Context, sub *subscriber) error {

	if !strings.HasPrefix(sub.url, "http") {
		subs, err := pubsub.OpenSubscription(ctx, sub.url)
		if err != nil {
			return fmt.Errorf("could not open topic subscription: %+v", err)
		}

		s.AddCleanupMethod(func() {
			err = subs.Shutdown(ctx)
			if err != nil {
				s.L().WithError(err).WithField("reference", sub.url).Info("subscription could not be stopped")

			}
		})

		sub.subscription = subs
	}
	sub.isInit = true
	return nil
}

func (s *Service) initPubsub(ctx context.Context) error {
	// Whenever the registry is not empty the events queue is automatically initiated
	if s.eventRegistry != nil && len(s.eventRegistry) > 0 {
		eventsQueueHandler := eventQueueHandler{
			service: s,
		}
		eventsQueueURL := GetEnv(envEventsQueueUrl, fmt.Sprintf("mem://%s", eventsQueueName))
		eventsQueue := RegisterSubscriber(eventsQueueName, eventsQueueURL, 10, &eventsQueueHandler)
		eventsQueue(s)
		eventsQueueP := RegisterPublisher(eventsQueueName, eventsQueueURL)
		eventsQueueP(s)
	}

	if s.queue == nil {
		return nil
	}

	for _, pub := range s.queue.publishQueueMap {
		err := s.initPublisher(ctx, pub)
		if err != nil {
			return err
		}
	}

	for _, sub := range s.queue.subscriptionQueueMap {
		err := s.initSubscriber(ctx, sub)
		if err != nil {
			return err
		}
	}

	if len(s.queue.subscriptionQueueMap) > 0 {
		s.subscribe(ctx)
	}

	return nil

}

func (s *Service) subscribe(ctx context.Context) {

	for _, subsc := range s.queue.subscriptionQueueMap { // cloud event subscriptions are not held as long running processes
		if strings.HasPrefix(subsc.url, "http") {
			continue
		}

		go func(localSub *subscriber) {
			sem := make(chan struct{}, localSub.concurrency)
		recvLoop:
			for {
				msg, err := localSub.subscription.Receive(ctx)
				if err != nil {
					// Errors from Receive indicate that Receive will no longer succeed.
					log.Printf(" subscribe -- Could not pull message because : %+v", err)
					localSub.isInit = false
					return
				}

				// Wait if there are too many active handle goroutines and acquire the
				// semaphore. If the context is canceled, stop waiting and start shutting
				// down.
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					break recvLoop
				}

				go func() {
					defer func() { <-sem }() // Release the semaphore.

					authClaim := ClaimsFromMap(msg.Metadata)

					if nil != authClaim {
						ctx = authClaim.ClaimsToContext(ctx)
					}

					ctx = ToContext(ctx, s)

					err := localSub.handler.Handle(ctx, msg.Body)
					if err != nil {
						log.Printf(" subscribe -- Unable to process message %v : %v",
							localSub.url, err)
						msg.Nack()
						return
					}

					msg.Ack()
				}()
			}

			// We're no longer receiving messages. Wait to finish handling any
			// unacknowledged messages by totally acquiring the semaphore.
			for n := 0; n < localSub.concurrency; n++ {
				sem <- struct{}{}
			}

			localSub.isInit = false

		}(subsc)
	}
}
