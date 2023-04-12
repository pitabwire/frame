package frame

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	"golang.org/x/sync/errgroup"
	"strings"
	"sync"
	"sync/atomic"
)

type queue struct {
	publishQueueMap      *sync.Map
	subscriptionQueueMap *sync.Map
}

func (q queue) getPublisherByReference(reference string) (*publisher, error) {
	p, ok := q.publishQueueMap.Load(reference)
	if !ok {
		return nil, fmt.Errorf("reference does not exist")
	}
	return p.(*publisher), nil
}

func newQueue() *queue {
	q := &queue{
		publishQueueMap:      &sync.Map{},
		subscriptionQueueMap: &sync.Map{},
	}

	return q
}

type publisher struct {
	reference string
	url       string
	topic     *pubsub.Topic
}

type SubscribeWorker interface {
	Handle(ctx context.Context, message []byte) error
}

type subscriber struct {
	ctx          context.Context
	logger       *logrus.Entry
	reference    string
	url          string
	concurrency  int
	handler      SubscribeWorker
	subscription *pubsub.Subscription
	isInit       atomic.Bool
}

func (s *subscriber) listen() error {

	egrp, ctx1 := errgroup.WithContext(s.ctx)
	msgChan := make(chan *pubsub.Message)

	egrp.Go(func() error {
		defer close(msgChan)

		for {

			select {
			case <-ctx1.Done():
				return ctx1.Err()
			default:
			}

			msg, err := s.subscription.Receive(ctx1)
			if err != nil {
				s.logger.WithError(err).Error(" could not pull message")
				s.isInit.Store(false)
				return err
			}

			msgChan <- msg
		}
	})

	for i := 0; i < s.concurrency; i++ {
		workerIndex := i

		egrp.Go(func() error {
			for msg := range msgChan {
				logger := s.logger.WithFields(logrus.Fields{
					"worker": workerIndex,
				})

				authClaim := ClaimsFromMap(msg.Metadata)

				var ctx2 context.Context
				if nil != authClaim {
					ctx2 = authClaim.ClaimsToContext(ctx1)
				} else {
					ctx2 = ctx1
				}

				err := s.handler.Handle(ctx2, msg.Body)
				if err != nil {
					logger.WithError(err).Error("unable to process message")
					msg.Nack()
					return err
				} else {
					msg.Ack()
				}
			}
			return nil
		})

	}

	err := egrp.Wait()

	s.isInit.Store(false)
	return err
}

// DequeueClient Option to register a background processing function that is initialized before running servers
// this function is maintained alive using the same error group as the servers so that if any exit earlier due to error
// all stop functioning
func DequeueClient(deque func() error) Option {
	return func(s *Service) {
		s.backGroundClient = deque
	}
}

// RegisterPublisher Option to register publishing path referenced within the system
func RegisterPublisher(reference string, queueURL string) Option {
	return func(s *Service) {
		s.queue.publishQueueMap.Store(reference, &publisher{
			reference: reference,
			url:       queueURL,
		})
	}
}

// RegisterSubscriber Option to register a new subscription handler
func RegisterSubscriber(reference string, queueURL string, concurrency int,
	handler SubscribeWorker) Option {
	return func(s *Service) {
		s.queue.subscriptionQueueMap.Store(reference, &subscriber{
			reference:   reference,
			url:         queueURL,
			concurrency: concurrency,
			handler:     handler,
		})
	}
}

func (s *Service) SubscriptionIsInitiated(path string) bool {
	sub, ok := s.queue.subscriptionQueueMap.Load(path)
	if !ok {
		return false
	}
	return sub.(*subscriber).isInit.Load()
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

		if !strings.Contains(reference, "://") {
			return err
		}

		pub = &publisher{
			reference: reference,
			url:       reference,
		}
		err = s.initPublisher(ctx, pub)
		if err != nil {
			return err
		}
		s.queue.publishQueueMap.Store(reference, pub)
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

	return pub.topic.Send(ctx, &pubsub.Message{
		Body:     message,
		Metadata: metadata,
	})

}

func (s *Service) initPublisher(ctx context.Context, pub *publisher) error {
	topic, err := pubsub.OpenTopic(ctx, pub.url)
	if err != nil {
		return err
	}
	s.AddCleanupMethod(func(ctx context.Context) {
		err = topic.Shutdown(ctx)
		if err != nil {
			s.L().WithError(err).WithField("reference", pub.reference).Warn("topic could not be closed")
		}
	})
	pub.topic = topic
	return nil
}
func (s *Service) initSubscriber(ctx context.Context, sub *subscriber) error {
	if !strings.HasPrefix(sub.url, "http") {
		subsc, err := pubsub.OpenSubscription(ctx, sub.url)
		if err != nil {
			return fmt.Errorf("could not open topic subscription: %+v", err)
		}

		s.AddCleanupMethod(func(ctx context.Context) {
			err = subsc.Shutdown(ctx)
			if err != nil {
				s.L().WithError(err).WithField("reference", sub.reference).Warn("subscription could not be stopped")
			}
		})

		sub.subscription = subsc
	}
	sub.isInit.Store(true)
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

	var publishQSlice []*publisher
	s.queue.publishQueueMap.Range(func(key, value any) bool {
		pub := value.(*publisher)
		publishQSlice = append(publishQSlice, pub)
		return true
	})

	for _, pub := range publishQSlice {
		err := s.initPublisher(ctx, pub)
		if err != nil {
			return err
		}
	}

	var subscribeQSlice []*subscriber
	s.queue.subscriptionQueueMap.Range(func(key, value any) bool {
		sub := value.(*subscriber)
		subscribeQSlice = append(subscribeQSlice, sub)
		return true
	})

	for _, sub := range subscribeQSlice {
		err := s.initSubscriber(ctx, sub)
		if err != nil {
			return err
		}
	}

	if len(subscribeQSlice) > 0 {
		s.subscribe(ctx)
	}

	return nil
}

func (s *Service) subscribe(ctx context.Context) {

	s.queue.subscriptionQueueMap.Range(func(key, value any) bool {

		subsc := value.(*subscriber)
		logger := s.L().WithField("subscriber", subsc.reference).WithField("url", subsc.url)

		if strings.HasPrefix(subsc.url, "http") {
			return true
		}
		subsc.ctx = ctx
		subsc.logger = logger
		s.errorGroup.Go(subsc.listen)
		return true
	})
}
