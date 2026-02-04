package events

import (
	"context"
	"errors"
	"sync"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/queue"
)

type manager struct {
	qm  queue.Manager
	cfg config.ConfigurationEvents

	mu            sync.RWMutex
	eventRegistry map[string]EventI
}

func (m *manager) Add(evt EventI) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventRegistry[evt.Name()] = evt
}

func (m *manager) Get(eventName string) (EventI, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	evt, ok := m.eventRegistry[eventName]
	if !ok {
		return nil, errors.New("event not found in registry: " + eventName)
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
	}
}
