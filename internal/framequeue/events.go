package framequeue

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/pitabwire/frame/internal/common"
)

type eventPayload struct {
	ID      string `json:",omitempty"`
	Name    string `json:",omitempty"`
	Payload string `json:",omitempty"`
}

// EventI an interface to represent a system event. All logic of an event is handled in the execute task
// and can also emit other events into the system or if they don't emit an event the processFunc is deemed complete.
type EventI interface {
	// Name represents the unique human readable id of the event that is used to pick it from the registry
	// or route follow up processing for system to processFunc using this particular event
	Name() string

	// PayloadType determines the type of payload the event uses. This is useful for decoding queue data.
	PayloadType() any

	// Validate enables automatic validation of payload supplied to the event without handling it in the execute block
	Validate(ctx context.Context, payload any) error

	// Execute performs all the logic required to action a step in the sequence of events required to achieve the end goal.
	Execute(ctx context.Context, payload any) error
}

// WithRegisterEvents registers events for the service. All events are unique and shouldn't share a name otherwise the last one registered will take precedence.
func WithRegisterEvents(events ...EventI) common.Option {
	return func(_ context.Context, s common.Service) {
		// Get eventRegistry from QueueModule
		module := s.GetModule(common.ModuleTypeQueue)
		if module == nil {
			return
		}
		
		queueModule, ok := module.(common.QueueModule)
		if !ok {
			return
		}
		
		eventRegistry := queueModule.EventRegistry()
		if eventRegistry == nil {
			eventRegistry = make(map[string]interface{})
		}

		for _, event := range events {
			eventRegistry[event.Name()] = event
		}
	}
}

// Emit method moved to avoid serviceImpl dependency

type eventQueueHandler struct {
	service common.Service
}

func (eq *eventQueueHandler) Handle(ctx context.Context, _ map[string]string, payload []byte) error {
	evtPyl := &eventPayload{}
	err := json.Unmarshal(payload, evtPyl)
	if err != nil {
		return err
	}

	// Get eventRegistry from QueueModule
	module := eq.service.GetModule(common.ModuleTypeQueue)
	if module == nil {
		eq.service.Log(ctx).WithField("event", evtPyl.Name).Error("Queue module not found")
		return errors.New("queue module not available")
	}
	
	queueModule, ok := module.(common.QueueModule)
	if !ok {
		eq.service.Log(ctx).WithField("event", evtPyl.Name).Error("Invalid queue module type")
		return errors.New("invalid queue module type")
	}
	
	eventRegistry := queueModule.EventRegistry()
	eventHandlerInterface, ok := eventRegistry[evtPyl.Name]
	if !ok {
		eq.service.Log(ctx).WithField("event", evtPyl.Name).Error("Could not get event from registry")
		return errors.New("event not found in registry")
	}

	eventHandler, ok := eventHandlerInterface.(EventI)
	if !ok {
		eq.service.Log(ctx).WithField("event", evtPyl.Name).Error("Invalid event handler type")
		return errors.New("invalid event handler type")
	}

	payLType := eventHandler.PayloadType()
	err = json.Unmarshal([]byte(evtPyl.Payload), payLType)
	if err != nil {
		return err
	}

	err = eventHandler.Validate(ctx, payLType)
	if err != nil {
		return err
	}

	err = eventHandler.Execute(ctx, payLType)
	if err != nil {
		return err
	}

	return nil
}
