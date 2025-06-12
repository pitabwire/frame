package frame

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"encoding/json"
	"sync/atomic"

	"gocloud.dev/pubsub"

	_ "github.com/pitabwire/natspubsub" // required for NATS pubsub driver registration
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
	Ref() string
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

func (p *publisher) Ref() string {
	return p.reference
}

func (p *publisher) Publish(ctx context.Context, payload any, headers ...map[string]string) error {
	var err error

	metadata := make(map[string]string)
	for _, h := range headers {
		maps.Copy(metadata, h)
	}

	authClaim := ClaimsFromContext(ctx)
	if authClaim != nil {
		maps.Copy(metadata, authClaim.AsMetadata())
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

const defaultPublisherShutdownTimeoutSeconds = 30

func (p *publisher) Stop(ctx context.Context) error {
	// TODO: incooporate trace information in shutdown context
	var sctx context.Context
	var cancelFunc context.CancelFunc

	select {
	case <-ctx.Done():
		sctx = context.Background()
	default:
		sctx = ctx
	}

	sctx, cancelFunc = context.WithTimeout(sctx, time.Second*defaultPublisherShutdownTimeoutSeconds)
	defer cancelFunc()

	p.isInit.Store(false)

	err := p.topic.Shutdown(sctx)
	if err != nil {
		return err
	}

	return nil
}

// WithRegisterPublisher Option to register publishing path referenced within the system.
func WithRegisterPublisher(reference string, queueURL string) Option {
	return func(_ context.Context, s *Service) {
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
	err := pub.Init(ctx)
	if err != nil {
		return err
	}

	s.queue.publishQueueMap.Store(reference, pub)
	return nil
}

func (s *Service) DiscardPublisher(ctx context.Context, reference string) error {
	var err error
	pub, _ := s.GetPublisher(reference)
	if pub != nil {
		err = pub.Stop(ctx)
	}

	s.queue.publishQueueMap.Delete(reference)
	return err
}

func (s *Service) GetPublisher(reference string) (Publisher, error) {
	pub, ok := s.queue.publishQueueMap.Load(reference)
	if !ok {
		return nil, fmt.Errorf("publisher %s not found", reference)
	}
	pVal, ok := pub.(*publisher)
	if !ok {
		return nil, fmt.Errorf("publisher %s is not of type *publisher", reference)
	}
	return pVal, nil
}

type Subscriber interface {
	Ref() string

	URI() string
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

	reference    string
	url          string
	handler      SubscribeWorker
	subscription *pubsub.Subscription
	isInit       atomic.Bool
	isIdle       atomic.Bool
}

func (s *subscriber) Ref() string {
	return s.reference
}

func (s *subscriber) URI() string {
	return s.url
}

func (s *subscriber) Receive(ctx context.Context) (*pubsub.Message, error) {
	msg, err := s.subscription.Receive(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			s.isIdle.Store(true)
		} else {
			s.isInit.Store(false)
		}
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
		subs, err := pubsub.OpenSubscription(ctx, s.url)
		if err != nil {
			return fmt.Errorf("could not open topic subscription: %w", err)
		}
		s.subscription = subs

		if s.handler != nil {
			job := NewJob(s.listen)

			err = SubmitJob(ctx, s.service, job)
			if err != nil {
				s.service.Log(ctx).WithField("subscriber", s.reference).WithField("url", s.url).
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
	// TODO: incooporate trace information in shutdown context
	var sctx context.Context
	var cancelFunc context.CancelFunc

	select {
	case <-ctx.Done():
		sctx = context.Background()
	default:
		sctx = ctx
	}

	sctx, cancelFunc = context.WithTimeout(sctx, time.Second*1)
	defer cancelFunc()

	s.isInit.Store(false)

	err := s.subscription.Shutdown(sctx)
	if err != nil {
		return err
	}

	return nil
}

func (s *subscriber) processReceivedMessage(ctx context.Context, msg *pubsub.Message) error {
	logger := s.service.Log(ctx).
		WithField("name", s.reference).
		WithField("function", "processReceivedMessage").
		WithField("url", s.url)

	job := NewJob(func(jobCtx context.Context, _ JobResultPipe[*pubsub.Message]) error {
		authClaim := ClaimsFromMap(msg.Metadata)
		var processedCtx context.Context
		if authClaim != nil {
			processedCtx = authClaim.ClaimsToContext(jobCtx)
		} else {
			processedCtx = jobCtx
		}

		handleErr := s.handler.Handle(processedCtx, msg.Metadata, msg.Body)
		if handleErr != nil {
			logger.WithError(handleErr).Warn("could not handle message")
			if msg.Nackable() {
				msg.Nack()
			}
			return handleErr // Propagate handler error to the job runner
		}
		msg.Ack()
		return nil
	})

	submitErr := SubmitJob(ctx, s.service, job) // Use the original listen context for submitting the job
	if submitErr != nil {
		// This error means the job submission itself failed, which is more critical.
		logger.WithError(submitErr).Error("failed to submit message processing job")
		// If job submission fails, the message might not have been acked/nacked properly
		// depending on where SubmitJob failed. Consider if nack is appropriate here
		// if the job func (and thus ack/nack) never ran.
		// However, Receive() would likely fetch it again if it's not acked.
		return submitErr
	}
	return nil
}

func (s *subscriber) listen(ctx context.Context, _ JobResultPipe[*pubsub.Message]) error {
	logger := s.service.Log(ctx).
		WithField("name", s.reference).
		WithField("function", "subscription").
		WithField("url", s.url)
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
					// Context cancelled or deadline exceeded, loop again to check ctx.Done()
					continue
				}
				// Other errors from Receive are critical for the listener.
				logger.WithError(err).Error("could not pull message")
				return err // Exit listen loop
			}

			// Process the received message. Errors from processing (like job submission failure)
			// will be logged by processReceivedMessage. If it's a critical submission error,
			// it will be returned and will stop the listener.
			if procErr := s.processReceivedMessage(ctx, msg); procErr != nil {
				// processReceivedMessage already logs details. This error is for critical failures.
				logger.WithError(procErr).Error("critical error processing message, stopping listener")
				return procErr
			}
		}
	}
}

// WithRegisterSubscriber Option to register a new subscription handler.
func WithRegisterSubscriber(reference string, queueURL string,
	handler ...SubscribeWorker) Option {
	return func(_ context.Context, s *Service) {
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

func (s *Service) AddSubscriber(
	ctx context.Context,
	reference string,
	queueURL string,
	handler ...SubscribeWorker,
) error {
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

func (s *Service) DiscardSubscriber(ctx context.Context, reference string) error {
	var err error
	sub, _ := s.GetSubscriber(reference)
	if sub != nil {
		err = sub.Stop(ctx)
	}

	s.queue.subscriptionQueueMap.Delete(reference)
	return err
}

func (s *Service) GetSubscriber(reference string) (Subscriber, error) {
	sub, ok := s.queue.subscriptionQueueMap.Load(reference)
	if !ok {
		return nil, fmt.Errorf("subscriber %s not found", reference)
	}
	sVal, ok := sub.(*subscriber)
	if !ok {
		return nil, fmt.Errorf("subscriber %s is not of type *subscriber", reference)
	}
	return sVal, nil
}

// Publish Queue method to write a new message into the queue pre initialized with the supplied reference.
func (s *Service) Publish(ctx context.Context, reference string, payload any, headers ...map[string]string) error {
	pub, err := s.GetPublisher(reference)
	if err != nil {
		return err
	}

	return pub.Publish(ctx, payload, headers...)
}

func (s *Service) initSubscriber(ctx context.Context, sub Subscriber) error {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	err := s.AddPublisher(ctx, sub.Ref(), sub.URI())
	if err != nil {
		return err
	}

	return sub.Init(ctx)
}

// setupEventsQueueIfNeeded sets up the default events queue publisher and subscriber
// if an event registry is configured for the service.
func (s *Service) setupEventsQueueIfNeeded(ctx context.Context) error {
	if len(s.eventRegistry) == 0 {
		return nil
	}

	eventsQueueHandler := eventQueueHandler{
		service: s,
	}

	config, ok := s.Config().(ConfigurationEvents)
	if !ok {
		errMsg := "configuration object does not implement ConfigurationEvents, cannot setup events queue"
		s.Log(ctx).Error(errMsg)
		return errors.New(errMsg)
	}

	eventsQueueSubscriberOpt := WithRegisterSubscriber(
		config.GetEventsQueueName(),
		config.GetEventsQueueURL(),
		&eventsQueueHandler,
	)
	eventsQueueSubscriberOpt(ctx, s) // This registers the subscriber

	eventsQueuePublisherOpt := WithRegisterPublisher(config.GetEventsQueueName(), config.GetEventsQueueURL())
	eventsQueuePublisherOpt(ctx, s) // This registers the publisher

	// Note: Actual initialization of this specific subscriber and publisher
	// will happen in initializeRegisteredPublishers and initializeRegisteredSubscribers.
	return nil
}

// initializeRegisteredPublishers iterates over and initializes all registered publishers.
func (s *Service) initializeRegisteredPublishers(ctx context.Context) error {
	var initErrors []error
	s.queue.publishQueueMap.Range(func(key, value any) bool {
		pub, ok := value.(*publisher)
		if !ok {
			s.Log(ctx).WithField("key", key).
				WithField("actual_type", fmt.Sprintf("%T", value)).
				Warn("Item in publishQueueMap is not of type *publisher, skipping initialization.")
			return true // continue to next item
		}
		if err := pub.Init(ctx); err != nil {
			s.Log(ctx).WithError(err).
				WithField("publisher_ref", pub.Ref()).
				WithField("publisher_url", pub.url).
				Error("Failed to initialize publisher")
			initErrors = append(initErrors, fmt.Errorf("publisher %s: %w", pub.Ref(), err))
		}
		return true
	})

	if len(initErrors) > 0 {
		// Consider how to aggregate multiple errors. For now, return the first one.
		// Or use a multierror package if available/preferred.
		return fmt.Errorf("failed to initialize one or more publishers: %w", initErrors[0])
	}
	return nil
}

// initializeRegisteredSubscribers iterates over and initializes all registered subscribers.
func (s *Service) initializeRegisteredSubscribers(ctx context.Context) error {
	var initErrors []error
	s.queue.subscriptionQueueMap.Range(func(key, value any) bool {
		sub, ok := value.(*subscriber)
		if !ok {
			s.Log(ctx).WithField("key", key).
				WithField("actual_type", fmt.Sprintf("%T", value)).
				Warn("Item in subscriptionQueueMap is not of type *subscriber, skipping initialization.")
			return true // continue to next item
		}
		if err := s.initSubscriber(ctx, sub); err != nil {
			s.Log(ctx).WithError(err).
				WithField("subscriber_ref", sub.Ref()).
				WithField("subscriber_url", sub.URI()).
				Error("Failed to initialize subscriber")
			initErrors = append(initErrors, fmt.Errorf("subscriber %s: %w", sub.Ref(), err))
		}
		return true
	})

	if len(initErrors) > 0 {
		return fmt.Errorf("failed to initialize one or more subscribers: %w", initErrors[0])
	}
	return nil
}

func (s *Service) initPubsub(ctx context.Context) error {
	if err := s.setupEventsQueueIfNeeded(ctx); err != nil {
		// Error already logged by helper
		return fmt.Errorf("failed to setup events queue: %w", err)
	}

	if s.queue == nil {
		s.Log(ctx).Debug(
			"No generic queue backend configured (s.queue is nil), skipping further pub/sub initialization.",
		)
		return nil
	}

	if err := s.initializeRegisteredPublishers(ctx); err != nil {
		// Errors logged by helper
		return fmt.Errorf("failed during publisher initialization: %w", err)
	}

	if err := s.initializeRegisteredSubscribers(ctx); err != nil {
		// Errors logged by helper
		return fmt.Errorf("failed during subscriber initialization: %w", err)
	}

	s.Log(ctx).Info("Pub/Sub system initialized successfully.")
	return nil
}
