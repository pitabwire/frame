package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
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
	Handle(ctx context.Context, message []byte) error
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

func (s *subscriber) listen(ctx context.Context) {

	service := FromContext(ctx)
	logger := service.L().WithField("name", s.reference).WithField("function", "subscription").WithField("url", s.url)
	logger.Debug("starting to listen for messages")
	for {

		select {
		case <-ctx.Done():
			s.isInit.Store(false)
			logger.Debug("exiting due to canceled context")
			return
		default:

			msg, err := s.subscription.Receive(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					continue
				}

				logger.WithError(err).Error(" could not pull message")
				s.isInit.Store(false)
				service.errorChannel <- err
				return
			}

			job := service.NewJob(func(ctx context.Context) error {
				authClaim := ClaimsFromMap(msg.Metadata)

				var ctx2 context.Context
				if nil != authClaim {
					ctx2 = authClaim.ClaimsToContext(ctx)
				} else {
					ctx2 = ctx
				}

				err0 := s.handler.Handle(ctx2, msg.Body)
				if err0 != nil {
					logger.WithError(err0).Warn(" could not handle message")
					return err0
				}
				msg.Ack()
				return nil
			}, 0)

			err = service.SubmitJob(ctx, job)
			if err != nil {
				logger.WithError(err).Warn(" ignoring handle error message")
				continue
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

	s.mu.Lock()
	defer s.mu.Unlock()

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

	s.mu.Lock()
	defer s.mu.Unlock()

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
	if s.eventRegistry != nil && len(s.eventRegistry) > 0 {
		eventsQueueHandler := eventQueueHandler{
			service: s,
		}

		config, ok := s.Config().(ConfigurationEvents)
		if !ok {
			s.L().Warn("configuration object not of type : ConfigurationDefault")
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

	s.queue.publishQueueMap.Range(func(key, value any) bool {
		pub := value.(*publisher)
		err := s.initPublisher(ctx, pub)
		if err != nil {
			s.errorChannel <- err
		}
		return true
	})

	s.queue.subscriptionQueueMap.Range(func(key, value any) bool {
		sub := value.(*subscriber)
		err := s.initSubscriber(ctx, sub)
		if err != nil {
			s.errorChannel <- err
		}
		return true
	})

	s.subscribe(ctx)

	return nil
}

func (s *Service) subscribe(ctx context.Context) {

	s.queue.subscriptionQueueMap.Range(func(key, value any) bool {

		subsc := value.(*subscriber)
		logger := s.L().WithField("subscriber", subsc.reference).WithField("url", subsc.url)

		if strings.HasPrefix(subsc.url, "http") {
			return true
		}
		subsc.logger = logger

		go subsc.listen(ctx)

		return true
	})
}
