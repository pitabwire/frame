package events

import (
	"context"
	"errors"

	"github.com/pitabwire/frame/queue"
)

type manager struct {
	eventRegistry map[string]EventI
}

func (m *manager) Add(evt EventI) {
	m.eventRegistry[evt.Name()] = evt
}

func (m *manager) Get(eventName string) (EventI, error) {
	evt, ok := m.eventRegistry[eventName]
	if !ok {
		return nil, errors.New("event not found in registry: " + eventName)
	}

	return evt, nil
}

func (m *manager) Handler() queue.SubscribeWorker {
	return &eventQueueHandler{
		manager: m,
	}
}

func NewManager(_ context.Context) Manager {
	return &manager{
		eventRegistry: make(map[string]EventI),
	}
}
