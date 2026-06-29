package events

import (
	"context"
	"sync"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/v2/config"
	"github.com/pitabwire/frame/v2/queue"
)

type manager struct {
	qm  queue.Manager
	cfg config.ConfigurationEvents

	mu            sync.RWMutex
	eventRegistry map[string]EventI
	strict        bool
}

func (m *manager) Add(evt EventI) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventRegistry[evt.Name()] = evt
}

// SetStrict toggles the unknown-event behaviour. Defaults to true
// (legacy fail-loud). Calling SetStrict(false) makes the queue handler
// ack-and-skip events whose name isn't in the registry, which the
// shared-stream consumer pattern needs to avoid wedging on sibling-
// consumer events.
func (m *manager) SetStrict(s bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.strict = s
}

func (m *manager) Strict() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.strict
}

func (m *manager) Get(eventName string) (EventI, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	evt, ok := m.eventRegistry[eventName]
	if !ok {
		return nil, ErrUnregisteredEvent
	}

	return evt, nil
}

// Emit publishes an event with the given name and payload to the event queue.
func (m *manager) Emit(ctx context.Context, name string, payload any) error {
	// Enqueue event message for further processing
	err := m.qm.
		Publish(ctx, m.cfg.GetEventsQueueName(), payload, map[string]string{EventHeaderName: name})
	if err != nil {
		util.Log(ctx).WithError(err).WithField("name", name).Error("Could not emit event")
		return err
	}

	return nil
}

func (m *manager) Handler() queue.SubscribeWorker {
	return &eventQueueHandler{
		manager: m,
	}
}

func NewManager(_ context.Context, qm queue.Manager, cfg config.ConfigurationEvents) Manager {
	return &manager{
		qm:            qm,
		cfg:           cfg,
		eventRegistry: make(map[string]EventI),
		// Default to strict — failing loud on unknown events is the
		// right safety net for any service with a closed handler set.
		// Catch-all-subject consumers opt out via SetStrict(false).
		strict: true,
	}
}
