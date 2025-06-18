package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/pitabwire/natspubsub" // required for NATS pubsub driver registration
	"github.com/pitabwire/util"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub" // required for in-memory pubsub driver registration
	"google.golang.org/protobuf/proto"
)

type SubscriberState int

const (
	SubscriberStateWaiting SubscriberState = iota
	SubscriberStateProcessing
	SubscriberStateInError
)

// SubscriberMetrics tracks operational metrics for a subscriber.
type SubscriberMetrics struct {
	ActiveMessages *atomic.Int64 // Currently active messages being processed
	LastActivity   *atomic.Int64 // Last activity timestamp in UnixNano
	ProcessingTime *atomic.Int64 // Total processing time in nanoseconds
	MessageCount   *atomic.Int64 // Total messages processed
	ErrorCount     *atomic.Int64 // Total number of errors encountered
}

// IsIdle and is in waiting state.
func (m *SubscriberMetrics) IsIdle(state SubscriberState) bool {
	return state == SubscriberStateWaiting && m.ActiveMessages.Load() <= 0
}

// IdleTime returns the duration since last activity if the subscriber is idle.
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

// AverageProcessingTime returns the average time spent processing messages.
func (m *SubscriberMetrics) AverageProcessingTime() time.Duration {
	count := m.MessageCount.Load()
	if count == 0 {
		return 0
	}

	return time.Duration(m.ProcessingTime.Load() / count)
}

func (m *SubscriberMetrics) closeMessage(startTime time.Time, err error) {
	if err != nil {
		m.ErrorCount.Add(1)
	}

	// Update metrics after processing
	m.ProcessingTime.Add(time.Since(startTime).Nanoseconds())
	m.MessageCount.Add(1)
	m.ActiveMessages.Add(-1)
	m.LastActivity.Store(time.Now().UnixNano())
}

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
	State() SubscriberState
	Metrics() *SubscriberMetrics
	IsIdle() bool

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
	handlers     []SubscribeWorker
	subscription *pubsub.Subscription
	isInit       atomic.Bool
	state        SubscriberState
	metrics      *SubscriberMetrics
}

func (s *subscriber) Ref() string {
	return s.reference
}

func (s *subscriber) URI() string {
	return s.url
}

func (s *subscriber) Receive(ctx context.Context) (*pubsub.Message, error) {
	if s.subscription == nil {
		return nil, errors.New("only initialised subscriptions can pull messages")
	}

	s.state = SubscriberStateWaiting
	s.metrics.LastActivity.Store(time.Now().UnixNano())

	msg, err := s.subscription.Receive(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		s.state = SubscriberStateInError
		s.metrics.ErrorCount.Add(1)
		return nil, err
	}
	s.state = SubscriberStateProcessing
	s.metrics.ActiveMessages.Add(1)
	return msg, nil
}

func (s *subscriber) createSubscription(ctx context.Context) error {
	if s.subscription != nil {
		return nil
	}

	if !strings.HasPrefix(s.url, "http") {
		subs, err := pubsub.OpenSubscription(ctx, s.url)
		if err != nil {
			return fmt.Errorf("could not open topic subscription: %w", err)
		}
		s.subscription = subs
	}

	return nil
}

func (s *subscriber) Init(ctx context.Context) error {
	if s.isInit.Load() && s.subscription != nil {
		return nil
	}

	err := s.createSubscription(ctx)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(s.url, "http") {
		if s.handlers != nil {
			go s.listen(ctx)
		}
	}

	s.isInit.Store(true)
	return nil
}

func (s *subscriber) recreateSubscription(ctx context.Context) {
	log := util.Log(ctx).WithField("subscriber", s.reference)

	if !s.isInit.Load() {
		log.Error("only initialised subscriptions can be recreated")
	}

	log.Warn("recreating subscription")

	if s.subscription != nil {
		err := s.subscription.Shutdown(ctx)
		if err != nil {
			log.WithError(err).Error("could not recreate subscription, stopping listener")
			s.service.sendStopError(ctx, err)
		}
		s.subscription = nil
	}

	err := s.createSubscription(ctx)
	if err != nil {
		log.WithError(err).Error("could not recreate subscription, stopping listener")
		s.service.sendStopError(ctx, err)
	}
}

func (s *subscriber) Initiated() bool {
	return s.isInit.Load()
}

func (s *subscriber) State() SubscriberState {
	return s.state
}

func (s *subscriber) Metrics() *SubscriberMetrics {
	return s.metrics
}

func (s *subscriber) IsIdle() bool {
	return s.metrics.IsIdle(s.state)
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

	if s.subscription != nil {
		err := s.subscription.Shutdown(sctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *subscriber) processReceivedMessage(ctx context.Context, msg *pubsub.Message) error {
	job := NewJob(func(jobCtx context.Context, _ JobResultPipe) error {
		var err error
		defer s.metrics.closeMessage(time.Now(), err)

		authClaim := ClaimsFromMap(msg.Metadata)
		var processedCtx context.Context
		if authClaim != nil {
			processedCtx = authClaim.ClaimsToContext(jobCtx)
		} else {
			processedCtx = jobCtx
		}

		for _, worker := range s.handlers {
			err = worker.Handle(processedCtx, msg.Metadata, msg.Body)
			if err != nil {
				logger := s.service.Log(processedCtx).
					WithField("name", s.reference).
					WithField("function", "processReceivedMessage").
					WithField("url", s.url)
				logger.WithError(err).Warn("could not handle message")
				msg.Nack()
				return err // Propagate handlers error to the job runner
			}
		}
		msg.Ack()
		return nil
	})

	submitErr := SubmitJob(ctx, s.service, job)
	if submitErr != nil {
		msg.Nack()
		logger := s.service.Log(ctx).
			WithField("name", s.reference).
			WithField("function", "processReceivedMessage").
			WithField("url", s.url)
		logger.WithError(submitErr).Error("could not process message, failed to submit job")
		s.metrics.closeMessage(time.Now(), submitErr)
		return submitErr
	}

	return nil
}

func (s *subscriber) listen(ctx context.Context) {
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
				return
			}
			logger.Debug("exiting due to canceled context")
			return

		default:
			msg, err := s.Receive(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					// Context cancelled or deadline exceeded, loop again to check ctx.Done()
					continue
				}
				// Other errors from Receive are critical for the listener.
				logger.WithError(err).Error("could not pull message")

				// Recreate subscription
				s.recreateSubscription(ctx)
				continue
			}

			// Process the received message. Errors from processing (like job submission failure)
			// will be logged by processReceivedMessage. If it's a critical submission error,
			// it will be returned and will stop the whole application.
			if procErr := s.processReceivedMessage(ctx, msg); procErr != nil {
				// processReceivedMessage already logs details. This error is for critical failures.
				logger.WithError(procErr).Error("critical error processing message, stopping listener")
				s.service.sendStopError(ctx, procErr) // procErr
				return                                // Exit listen loop
			}
		}
	}
}

func newSubscriber(s *Service, reference string, queueURL string, handlers ...SubscribeWorker) *subscriber {
	return &subscriber{
		service:   s,
		reference: reference,
		url:       queueURL,
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

// WithRegisterSubscriber Option to register a new subscription handlers.
func WithRegisterSubscriber(reference string, queueURL string,
	handlers ...SubscribeWorker) Option {
	return func(_ context.Context, s *Service) {
		subs := newSubscriber(s, reference, queueURL, handlers...)
		s.queue.subscriptionQueueMap.Store(reference, subs)
	}
}

func (s *Service) AddSubscriber(
	ctx context.Context,
	reference string,
	queueURL string,
	handlers ...SubscribeWorker,
) error {
	subs0, _ := s.GetSubscriber(reference)
	if subs0 != nil {
		return nil
	}

	subs := newSubscriber(s, reference, queueURL, handlers...)
	err := s.initSubscriber(ctx, subs)
	if err != nil {
		return err
	}

	s.queue.subscriptionQueueMap.Store(reference, subs)

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
