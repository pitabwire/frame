package frame

import (
	"context"
	"encoding/json"
	"errors"
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
	//or route follow up processing for system to processFunc using this particular event
	Name() string

	// PayloadType determines the type of payload the event uses. This is useful for decoding queue data.
	PayloadType() any

	// Validate enables automatic validation of payload supplied to the event without handling it in the execute block
	Validate(ctx context.Context, payload any) error

	// Execute performs all the logic required to action a step in the sequence of events required to achieve the end goal.
	Execute(ctx context.Context, payload any) error
}

// WithRegisterEvents Option to write an event or list of events into the service registry for future use.
// All events are unique and shouldn't share a name otherwise the last one registered will take presedence
func WithRegisterEvents(events ...EventI) Option {
	return func(ctx context.Context, s *Service) {

		if s.eventRegistry == nil {
			s.eventRegistry = make(map[string]EventI)
		}

		for _, event := range events {
			s.eventRegistry[event.Name()] = event
		}

	}
}

// Emit a simple method used to deploy
func (s *Service) Emit(ctx context.Context, name string, payload any) error {

	payloadBytes, err := json.Marshal(payload)

	if err != nil {
		return err
	}

	e := eventPayload{Name: name, Payload: string(payloadBytes)}

	config, ok := s.Config().(ConfigurationEvents)
	if !ok {
		s.Log(ctx).Warn("configuration object not of type : ConfigurationDefault")
		return errors.New("could not cast config to ConfigurationEvents")
	}

	// Queue event message for further processing
	err = s.Publish(ctx, config.GetEventsQueueName(), e)
	if err != nil {
		s.Log(ctx).WithError(err).WithField("name", name).Error("Could not emit event")
		return err
	}

	return nil
}

type eventQueueHandler struct {
	service *Service
}

func (eq *eventQueueHandler) Handle(ctx context.Context, header map[string]string, payload []byte) error {

	evtPyl := &eventPayload{}
	err := json.Unmarshal(payload, evtPyl)
	if err != nil {
		return err
	}

	eventHandler, ok := eq.service.eventRegistry[evtPyl.Name]
	if !ok {
		eq.service.Log(ctx).WithField("event", evtPyl.Name).Error("Could not get event from registry")
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
