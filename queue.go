package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/pitabwire/natspubsub"
	"github.com/sirupsen/logrus"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	"maps"
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

func newQueue(_ context.Context) *queue {
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
	Handle(ctx context.Context, metadata map[string]string, message []byte) error
}

type subscriber struct {
	logger *logrus.Entry

	reference    string
	url          string
	concurrency  int
	handler      SubscribeWorker
	subscription *pubsub.Subscription
	isInit       atomic.Bool
}

func (s *subscriber) listen(ctx context.Context, _ JobResultPipe[*pubsub.Message]) error {

	service := FromContext(ctx)
	logger := service.L(ctx).WithField("name", s.reference).WithField("function", "subscription").WithField("url", s.url)
	logger.Debug("starting to listen for messages")
	for {

		select {
		case <-ctx.Done():
			s.isInit.Store(false)
			logger.Debug("exiting due to canceled context")
			return ctx.Err()

		default:

			msg, err := s.subscription.Receive(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					continue
				}

				logger.WithError(err).Error(" could not pull message")
				s.isInit.Store(false)
				return err
			}

			job := NewJob(func(ctx context.Context, _ JobResultPipe[*pubsub.Message]) error {
				authClaim := ClaimsFromMap(msg.Metadata)

				var ctx2 context.Context
				if nil != authClaim {
					ctx2 = authClaim.ClaimsToContext(ctx)
				} else {
					ctx2 = ctx
				}

				err0 := s.handler.Handle(ctx2, msg.Metadata, msg.Body)
				if err0 != nil {
					logger.WithError(err0).Warn(" could not handle message")
					if msg.Nackable() {
						msg.Nack()
					}
					return err0
				}
				msg.Ack()
				return nil
			})

			err = SubmitJob(ctx, service, job)
			if err != nil {
				logger.WithError(err).Warn(" Ignoring handle error message")
				return err
			}

		}
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

func (s *Service) IsPublisherRegistered(_ context.Context, reference string) bool {
	_, ok := s.queue.publishQueueMap.Load(reference)
	return ok
}

func (s *Service) AddPublisher(ctx context.Context, reference string, queueURL string) error {

	if s.IsPublisherRegistered(ctx, reference) {
		return nil
	}

	pub := &publisher{
		reference: reference,
		url:       queueURL,
	}
	err := s.initPublisher(ctx, pub)
	if err != nil {
		return err
	}

	s.queue.publishQueueMap.Store(reference, pub)
	return nil
}

// Publish Queue method to write a new message into the queue pre initialized with the supplied reference
func (s *Service) Publish(ctx context.Context, reference string, payload any, headers ...map[string]string) error {

	metadata := make(map[string]string)
	for _, h := range headers {
		maps.Copy(metadata, h)
	}

	authClaim := ClaimsFromContext(ctx)
	if authClaim != nil {
		maps.Copy(metadata, authClaim.AsMetadata())
	}

	pub, err := s.queue.getPublisherByReference(reference)
	if err != nil {
		return err
	}

	var message []byte
	msg, ok := payload.([]byte)
	if !ok {
		msg0, err0 := json.Marshal(payload)
		if err0 != nil {
			return err
		}
		message = msg0
	} else {
		message = msg
	}

	topic := pub.topic

	return topic.Send(ctx, &pubsub.Message{
		Body:     message,
		Metadata: metadata,
	})

}

func (s *Service) initPublisher(ctx context.Context, pub *publisher) error {

	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	if pub.topic != nil {
		return nil
	}

	topic, err := pubsub.OpenTopic(ctx, pub.url)
	if err != nil {
		return err
	}

	pub.topic = topic

	return nil
}
func (s *Service) initSubscriber(ctx context.Context, sub *subscriber) error {

	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	if sub.isInit.Load() && sub.subscription != nil {
		return nil
	}

	if !strings.HasPrefix(sub.url, "http") {

		subsc, err := pubsub.OpenSubscription(ctx, sub.url)
		if err != nil {
			return fmt.Errorf("could not open topic subscription: %s", err)
		}
		sub.subscription = subsc
	}

	sub.isInit.Store(true)
	return nil
}

func (s *Service) initPubsub(ctx context.Context) error {
	// Whenever the registry is not empty the events queue is automatically initiated
	if len(s.eventRegistry) > 0 {
		eventsQueueHandler := eventQueueHandler{
			service: s,
		}

		config, ok := s.Config().(ConfigurationEvents)
		if !ok {
			s.L(ctx).Warn("configuration object not of type : ConfigurationDefault")
			return errors.New("could not cast config to ConfigurationEvents")
		}

		eventsQueue := RegisterSubscriber(config.GetEventsQueueName(), config.GetEventsQueueUrl(), 10, &eventsQueueHandler)
		eventsQueue(s)
		eventsQueueP := RegisterPublisher(config.GetEventsQueueName(), config.GetEventsQueueUrl())
		eventsQueueP(s)
	}

	if s.queue == nil {
		return nil
	}

	var publishers []*publisher

	s.queue.publishQueueMap.Range(func(key, value any) bool {
		pub := value.(*publisher)
		publishers = append(publishers, pub)
		return true
	})

	for _, pub := range publishers {
		err := s.initPublisher(ctx, pub)
		if err != nil {
			return err
		}
	}

	var subscribers []*subscriber

	s.queue.subscriptionQueueMap.Range(func(key, value any) bool {
		sub := value.(*subscriber)
		subscribers = append(subscribers, sub)
		return true
	})

	for _, sub := range subscribers {
		err := s.initSubscriber(ctx, sub)
		if err != nil {
			return err
		}
	}

	s.subscribe(ctx)

	return nil
}

func (s *Service) subscribe(ctx context.Context) {

	s.queue.subscriptionQueueMap.Range(func(key, value any) bool {

		subsc := value.(*subscriber)
		logger := s.L(ctx).WithField("subscriber", subsc.reference).WithField("url", subsc.url)

		if strings.HasPrefix(subsc.url, "http") {
			return true
		}
		subsc.logger = logger

		job := NewJob(subsc.listen)

		err := SubmitJob(ctx, s, job)
		if err != nil {
			logger.WithError(err).WithField("subscriber", subsc).Error(" could not listen or subscribe for messages")
			return false
		}

		return true
	})
}
