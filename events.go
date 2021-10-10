package frame

import (
	"context"
	"encoding/json"
	"log"
)


const eventsQueueName = "frame.events.internal_._queue"
const envEventsQueueUrl = "EVENTS_QUEUE_URL"

type eventPayload struct {
	ID string `json:",omitempty"`
	Name string `json:",omitempty"`
	Payload string `json:",omitempty"`
}

// EventI an interface to represent a system event. All logic of an event is handled in the execute task
//and can also emit other events into the system or if they don't emit an event the process is deemed complete.
type EventI interface {
	// Name represents the unique human readable id of the event that is used to pick it from the registry
	//or route follow up processing for system to process using this particular event
	Name() string

	// PayloadType determines the type of payload the event uses. This is useful for decoding queue data.
	PayloadType() interface{}

	// Validate enables automatic validation of payload supplied to the event without handling it in the execute block
	Validate(ctx context.Context, payload interface{}) error

	// Execute performs all the logic required to action a step in the sequence of events required to achieve the end goal.
	Execute(ctx context.Context, payload interface{}) error
}

// RegisterEvents Option to write an event or list of events into the service registry for future use.
//All events are unique and shouldn't share a name otherwise the last one registered will take presedence
func RegisterEvents(events ... EventI) Option {
	return func(s *Service) {

		if s.eventRegistry == nil {
			s.eventRegistry = make(map[string]EventI)
		}

		for _, event := range events {
			s.eventRegistry[event.Name()] = event
		}

	}
}


// Emit a simple method used to deploy
func (s *Service) Emit(ctx context.Context, name string, payload interface{}) error {

	payloadBytes, err := json.Marshal(payload)

	if err != nil {
		return err
	}

	e := eventPayload{Name: name, Payload: string(payloadBytes)}

	// Queue event message for further processing
	err = s.Publish(ctx, eventsQueueName, e)
	if err != nil {
		log.Printf("Could not emit event %s : -> %+v", name, err)
		return err
	}

	return nil
}

type eventQueueHandler struct {
	service    *Service
}

func (eq *eventQueueHandler) Handle(ctx context.Context, payload []byte) error {

	evtPyl := &eventPayload{}
	err := json.Unmarshal(payload, evtPyl)
	if err != nil {
		return err
	}

	eventHandler, ok := eq.service.eventRegistry[evtPyl.Name]
	if !ok {
		eq.service.L().Error("Could not get event %s from registry", evtPyl.Name)
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

