package queue

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/pitabwire/util"
	"golang.org/x/oauth2"

	"github.com/pitabwire/frame/v2/data"
	"github.com/pitabwire/frame/v2/queue/protocol"
	"github.com/pitabwire/frame/v2/workerpool"
)

type queueManager struct {
	stopMutex            sync.Mutex
	publishQueueMap      *sync.Map
	subscriptionQueueMap *sync.Map
	initialized          bool
	initMutex            sync.Mutex

	workPool    workerpool.Manager
	httpClient  *http.Client
	tokenSource oauth2.TokenSource
	serviceName string

	// Claim trust defaults applied to new subscribers (Secure Profile).
	claimTrust        protocol.ClaimTrustLevel
	honorInternalSkip bool
}

// ManagerOption configures the queue manager.
type ManagerOption func(*queueManager)

// WithHTTPClient sets the shared HTTP client for CE / Cloud Tasks publishers.
func WithHTTPClient(c *http.Client) ManagerOption {
	return func(m *queueManager) {
		m.httpClient = c
	}
}

// WithTokenSource sets the OAuth2 token source for Cloud Tasks CreateTask API calls.
func WithTokenSource(ts oauth2.TokenSource) ManagerOption {
	return func(m *queueManager) {
		m.tokenSource = ts
	}
}

// WithServiceName sets the default CE source host component.
func WithServiceName(name string) ManagerOption {
	return func(m *queueManager) {
		m.serviceName = name
	}
}

// WithClaimTrust sets the default ClaimTrustLevel for new subscribers.
// Default is TrustAll (legacy). Secure Profile uses TrustTenancyOnly.
func WithClaimTrust(level protocol.ClaimTrustLevel) ManagerOption {
	return func(m *queueManager) {
		m.claimTrust = level
	}
}

// WithHonorInternalSkip controls whether roles=internal in metadata maps
// to tenancy Skip. Default true. Secure Profile sets false.
func WithHonorInternalSkip(v bool) ManagerOption {
	return func(m *queueManager) {
		m.honorInternalSkip = v
	}
}

func NewQueueManager(_ context.Context, workPool workerpool.Manager, opts ...ManagerOption) Manager {
	q := &queueManager{
		publishQueueMap:      &sync.Map{},
		subscriptionQueueMap: &sync.Map{},
		workPool:             workPool,
		claimTrust:           protocol.TrustAll,
		honorInternalSkip:    true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(q)
		}
	}
	return q
}

func (s *queueManager) AddPublisher(ctx context.Context, reference string, queueURL string) error {
	if strings.TrimSpace(reference) == "" {
		return errors.New("publisher reference cannot be empty")
	}
	if !data.DSN(queueURL).Valid() {
		return errors.New("publisher queueURL cannot be empty")
	}

	pub, _ := s.GetPublisher(reference)
	if pub != nil {
		return nil
	}

	p := newPublisher(reference, queueURL)
	p.httpClient = s.httpClient
	p.tokenSource = s.tokenSource
	p.serviceName = s.serviceName

	s.initMutex.Lock()
	alreadyInitialized := s.initialized
	if alreadyInitialized {
		err := p.Init(ctx)
		if err != nil {
			s.initMutex.Unlock()
			return err
		}
	}
	s.publishQueueMap.Store(reference, p)
	s.initMutex.Unlock()
	return nil
}

func (s *queueManager) DiscardPublisher(ctx context.Context, reference string) error {
	var err error
	pub, _ := s.GetPublisher(reference)
	if pub != nil {
		err = pub.Stop(ctx)
	}

	s.publishQueueMap.Delete(reference)
	return err
}

func (s *queueManager) GetPublisher(reference string) (Publisher, error) {
	pub, ok := s.publishQueueMap.Load(reference)
	if !ok {
		return nil, fmt.Errorf("publisher %s not found", reference)
	}
	pVal, ok := pub.(*publisher)
	if !ok {
		return nil, fmt.Errorf("publisher %s is not of type *publisher", reference)
	}
	return pVal, nil
}

func (s *queueManager) AddSubscriber(
	ctx context.Context,
	reference string,
	queueURL string,
	handlers ...SubscribeWorker,
) error {
	if strings.TrimSpace(reference) == "" {
		return errors.New("subscriber reference cannot be empty")
	}
	if !data.DSN(queueURL).Valid() {
		return errors.New("subscriber queueURL cannot be empty")
	}

	subs0, _ := s.GetSubscriber(reference)
	if subs0 != nil {
		return nil
	}

	subs := newSubscriber(s.workPool, reference, queueURL, handlers...)
	subs.claimTrust = s.claimTrust
	subs.honorInternalSkip = s.honorInternalSkip

	s.initMutex.Lock()
	alreadyInitialized := s.initialized
	if alreadyInitialized {
		err := s.initSubscriber(ctx, subs)
		if err != nil {
			s.initMutex.Unlock()
			return err
		}
	}
	s.subscriptionQueueMap.Store(reference, subs)
	s.initMutex.Unlock()

	return nil
}

func (s *queueManager) DiscardSubscriber(ctx context.Context, reference string) error {
	var err error
	sub, _ := s.GetSubscriber(reference)
	if sub != nil {
		err = sub.Stop(ctx)
	}

	s.subscriptionQueueMap.Delete(reference)
	return err
}

func (s *queueManager) GetSubscriber(reference string) (Subscriber, error) {
	sub, ok := s.subscriptionQueueMap.Load(reference)
	if !ok {
		return nil, fmt.Errorf("subscriber %s not found", reference)
	}
	sVal, ok := sub.(*subscriber)
	if !ok {
		return nil, fmt.Errorf("subscriber %s is not of type *subscriber", reference)
	}
	return sVal, nil
}

func (s *queueManager) HasPushSubscribers() bool {
	found := false
	s.subscriptionQueueMap.Range(func(_, value any) bool {
		sub, ok := value.(*subscriber)
		if !ok {
			return true
		}
		if sub.isInit.Load() {
			if sub.Mode() == DeliveryModePush {
				found = true
				return false
			}
			return true
		}
		if mode, _, err := ClassifySubscriberURL(sub.url); err == nil && mode == DeliveryModePush {
			found = true
			return false
		}
		return true
	})
	return found
}

func (s *queueManager) LookupPush(ref string) (PushTarget, bool) {
	sub, err := s.GetSubscriber(ref)
	if err != nil {
		return nil, false
	}
	if sub.Mode() != DeliveryModePush {
		return nil, false
	}
	return sub, true
}

// Publish writes a new message into the queueManager pre-initialized publisher.
func (s *queueManager) Publish(ctx context.Context, reference string, payload any, headers ...map[string]string) error {
	pub, err := s.GetPublisher(reference)
	if err != nil {
		return err
	}

	return pub.Publish(ctx, payload, headers...)
}

func (s *queueManager) initSubscriber(ctx context.Context, sub Subscriber) error {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	return sub.Init(ctx)
}

func (s *queueManager) initializeRegisteredPublishers(ctx context.Context) error {
	var initErrors []error
	s.publishQueueMap.Range(func(key, value any) bool {
		pub, ok := value.(*publisher)
		if !ok {
			util.Log(ctx).WithField("key", key).
				WithField("actual_type", fmt.Sprintf("%T", value)).
				Warn("Item in publishQueueMap is not of type *publisher, skipping initialization.")
			return true
		}
		// refresh deps in case options set after construction
		pub.httpClient = s.httpClient
		pub.tokenSource = s.tokenSource
		pub.serviceName = s.serviceName
		if err := pub.Init(ctx); err != nil {
			util.Log(ctx).WithError(err).
				WithField("publisher_ref", pub.Ref()).
				WithField("publisher_url", pub.url).
				Error("Failed to initialize publisher")
			initErrors = append(initErrors, fmt.Errorf("publisher %s: %w", pub.Ref(), err))
		}
		return true
	})

	if len(initErrors) > 0 {
		return fmt.Errorf("failed to initialize one or more publishers: %w", initErrors[0])
	}
	return nil
}

func (s *queueManager) initializeRegisteredSubscribers(ctx context.Context) error {
	var initErrors []error
	s.subscriptionQueueMap.Range(func(key, value any) bool {
		sub, ok := value.(Subscriber)
		if !ok {
			util.Log(ctx).WithField("key", key).
				WithField("actual_type", fmt.Sprintf("%T", value)).
				Warn("Item in subscriptionQueueMap is not of type Subscriber, skipping initialization.")
			return true
		}
		if err := s.initSubscriber(ctx, sub); err != nil {
			util.Log(ctx).WithError(err).
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

func (s *queueManager) Init(ctx context.Context) error {
	if s == nil {
		util.Log(ctx).Debug(
			"No generic queueManager backend configured (s.queueManager is nil), skipping further pub/sub initialization.",
		)
		return nil
	}

	if err := s.initializeRegisteredPublishers(ctx); err != nil {
		return fmt.Errorf("failed during publisher initialization: %w", err)
	}

	if err := s.initializeRegisteredSubscribers(ctx); err != nil {
		return fmt.Errorf("failed during subscriber initialization: %w", err)
	}

	s.initMutex.Lock()
	s.initialized = true
	s.initMutex.Unlock()

	util.Log(ctx).Info("Pub/Sub system initialized successfully.")
	return nil
}

func (s *queueManager) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}

	var closeErr error

	s.publishQueueMap.Range(func(key, value any) bool {
		pub, ok := value.(*publisher)
		if !ok {
			s.publishQueueMap.Delete(key)
			return true
		}

		if err := pub.Stop(ctx); err != nil && closeErr == nil {
			closeErr = err
		}

		s.publishQueueMap.Delete(key)
		return true
	})

	s.subscriptionQueueMap.Range(func(key, value any) bool {
		sub, ok := value.(*subscriber)
		if !ok {
			s.subscriptionQueueMap.Delete(key)
			return true
		}

		if err := sub.Stop(ctx); err != nil && closeErr == nil {
			closeErr = err
		}

		s.subscriptionQueueMap.Delete(key)
		return true
	})

	s.initMutex.Lock()
	s.initialized = false
	s.initMutex.Unlock()

	return closeErr
}

// ListPublishers implements Inspector.
func (s *queueManager) ListPublishers() []PublisherInfo {
	var out []PublisherInfo
	s.publishQueueMap.Range(func(_, value any) bool {
		pub, ok := value.(*publisher)
		if !ok {
			return true
		}
		out = append(out, PublisherInfo{
			Reference: pub.Ref(),
			URL:       pub.url,
			Initiated: pub.Initiated(),
		})
		return true
	})
	return out
}

// ListSubscribers implements Inspector.
func (s *queueManager) ListSubscribers() []SubscriberInfo {
	var out []SubscriberInfo
	s.subscriptionQueueMap.Range(func(_, value any) bool {
		sub, ok := value.(*subscriber)
		if !ok {
			return true
		}
		out = append(out, SubscriberInfo{
			Reference: sub.Ref(),
			URL:       sub.URI(),
			State:     sub.State(),
			Initiated: sub.Initiated(),
			Mode:      sub.Mode(),
		})
		return true
	})
	return out
}
