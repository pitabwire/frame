package frame

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"

	"encoding/json"
	"sync/atomic"

	"gocloud.dev/pubsub"

	_ "github.com/pitabwire/natspubsub"
	_ "gocloud.dev/pubsub/mempubsub"
)

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

type Publisher interface {
	Initiated() bool

	Init(ctx context.Context) error

	Publish(ctx context.Context, payload any, headers ...map[string]string) error
	Stop(ctx context.Context) error
}

type publisher struct {
	reference string
	url       string
	topic     *pubsub.Topic
	isInit    atomic.Bool
}

func (p *publisher) Publish(ctx context.Context, payload any, headers ...map[string]string) error {
	metadata := make(map[string]string)
	for _, h := range headers {
		maps.Copy(metadata, h)
	}

	authClaim := ClaimsFromContext(ctx)
	if authClaim != nil {
		maps.Copy(metadata, authClaim.AsMetadata())
	}

	var message []byte
	msg, ok := payload.([]byte)
	if !ok {
		msg0, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		message = msg0
	} else {
		message = msg
	}

	topic := p.topic

	return topic.Send(ctx, &pubsub.Message{
		Body:     message,
		Metadata: metadata,
	})

}

func (p *publisher) Init(ctx context.Context) error {

	if p.isInit.Load() && p.topic != nil {
		return nil
	}

	var err error

	p.topic, err = pubsub.OpenTopic(ctx, p.url)
	if err != nil {
		return err
	}

	p.isInit.Store(true)
	return nil
}

func (p *publisher) Initiated() bool {
	return p.isInit.Load()
}

func (p *publisher) Stop(ctx context.Context) error {

	p.isInit.Store(false)

	err := p.topic.Shutdown(ctx)
	if err != nil {
		return err
	}

	return nil
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

func (s *Service) AddPublisher(ctx context.Context, reference string, queueURL string) error {

	pub, _ := s.GetPublisher(reference)
	if pub != nil {
		return nil
	}

	pub = &publisher{
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

func (s *Service) GetPublisher(reference string) (Publisher, error) {
	pub, ok := s.queue.publishQueueMap.Load(reference)
	if !ok {
		return nil, fmt.Errorf("publisher %s not found", reference)
	}
	return pub.(*publisher), nil
}

type Subscriber interface {
	Initiated() bool
	Idle() bool

	Init(ctx context.Context) error
	Receive(ctx context.Context) (*pubsub.Message, error)
	Stop(ctx context.Context) error
}

type SubscribeWorker interface {
	Handle(ctx context.Context, metadata map[string]string, message []byte) error
}

type subscriber struct {
	service *Service

	reference       string
	url             string
	handler         SubscribeWorker
	subscription    *pubsub.Subscription
	subscribedTopic *pubsub.Topic
	isInit          atomic.Bool
	isIdle          atomic.Bool
}

func (s *subscriber) Receive(ctx context.Context) (*pubsub.Message, error) {
	msg, err := s.subscription.Receive(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			s.isIdle.Store(true)
		}

		s.isInit.Store(false)
		return nil, err
	}
	s.isIdle.Store(false)
	return msg, err
}

func (s *subscriber) Init(ctx context.Context) error {
	if s.isInit.Load() && s.subscription != nil {
		return nil
	}

	if !strings.HasPrefix(s.url, "http") {

		if strings.HasPrefix(s.url, "mem://") {
			var err error
			s.subscribedTopic, err = pubsub.OpenTopic(ctx, s.url)
			if err != nil {
				return err
			}
		}

		subs, err := pubsub.OpenSubscription(ctx, s.url)
		if err != nil {
			return fmt.Errorf("could not open topic subscription: %s", err)
		}
		s.subscription = subs

		if s.handler != nil {

			job := NewJob(s.listen)

			err = SubmitJob(ctx, s.service, job)
			if err != nil {
				s.service.L(ctx).WithField("subscriber", s.reference).WithField("url", s.url).
					WithError(err).WithField("subscriber", subs).Error(" could not listen or subscribe for messages")
				return err
			}
		}
	}

	s.isInit.Store(true)
	return nil
}

func (s *subscriber) Initiated() bool {
	return s.isInit.Load()
}

func (s *subscriber) Idle() bool {
	return s.isIdle.Load()
}

func (s *subscriber) Stop(ctx context.Context) error {

	s.isInit.Store(false)

	if s.subscribedTopic != nil {
		err := s.subscribedTopic.Shutdown(ctx)
		if err != nil {
			return err
		}
	}

	err := s.subscription.Shutdown(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (s *subscriber) listen(ctx context.Context, _ JobResultPipe[*pubsub.Message]) error {

	logger := s.service.L(ctx).WithField("name", s.reference).WithField("function", "subscription").WithField("url", s.url)
	logger.Debug("starting to listen for messages")
	for {

		select {
		case <-ctx.Done():
			err := s.Stop(ctx)
			if err != nil {
				logger.WithError(err).Error("could not stop subscription")
				return err
			}
			logger.Debug("exiting due to canceled context")
			return ctx.Err()

		default:

			msg, err := s.Receive(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					continue
				}

				logger.WithError(err).Error(" could not pull message")
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

			err = SubmitJob(ctx, s.service, job)
			if err != nil {
				logger.WithError(err).Warn(" Ignoring handle error message")
				return err
			}

		}
	}
}

// RegisterSubscriber Option to register a new subscription handler
func RegisterSubscriber(reference string, queueURL string,
	handler ...SubscribeWorker) Option {
	return func(s *Service) {

		subs := subscriber{
			service:   s,
			reference: reference,
			url:       queueURL,
		}

		if len(handler) > 0 {
			subs.handler = handler[0]
		}

		s.queue.subscriptionQueueMap.Store(reference, &subs)
	}
}

func (s *Service) AddSubscriber(ctx context.Context, reference string, queueURL string, handler ...SubscribeWorker) error {

	subs0, _ := s.GetSubscriber(reference)
	if subs0 != nil {
		return nil
	}

	subs := subscriber{
		service:   s,
		reference: reference,
		url:       queueURL,
	}

	if len(handler) > 0 {
		subs.handler = handler[0]
	}

	err := s.initSubscriber(ctx, &subs)
	if err != nil {
		return err
	}

	s.queue.subscriptionQueueMap.Store(reference, &subs)

	return nil
}

func (s *Service) GetSubscriber(reference string) (Subscriber, error) {
	sub, ok := s.queue.subscriptionQueueMap.Load(reference)
	if !ok {
		return nil, fmt.Errorf("subscriber %s not found", reference)
	}
	return sub.(*subscriber), nil
}

// Publish Queue method to write a new message into the queue pre initialized with the supplied reference
func (s *Service) Publish(ctx context.Context, reference string, payload any, headers ...map[string]string) error {

	pub, err := s.GetPublisher(reference)
	if err != nil {
		return err
	}

	return pub.Publish(ctx, payload, headers...)
}

func (s *Service) initPublisher(ctx context.Context, pub Publisher) error {

	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	return pub.Init(ctx)
}

func (s *Service) initSubscriber(ctx context.Context, sub Subscriber) error {

	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	return sub.Init(ctx)
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

		eventsQueue := RegisterSubscriber(config.GetEventsQueueName(), config.GetEventsQueueUrl(), &eventsQueueHandler)
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

	return nil
}
