package frame

import (
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/proto"
)

const eventHeaderName = "frame._internal.event.header"

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
func WithRegisterEvents(events ...EventI) Option {
	return func(_ context.Context, s *Service) {
		if s.eventRegistry == nil {
			s.eventRegistry = make(map[string]EventI)
		}

		for _, event := range events {
			s.eventRegistry[event.Name()] = event
		}
	}
}

// Emit a simple method used to deploy.
func (s *Service) Emit(ctx context.Context, name string, payload any) error {
	config, ok := s.Config().(ConfigurationEvents)
	if !ok {
		s.Log(ctx).Warn("configuration object not of type : ConfigurationDefault")
		return errors.New("could not cast config to ConfigurationEvents")
	}

	// ByIsQueue event message for further processing
	err := s.Publish(ctx, config.GetEventsQueueName(), payload, map[string]string{eventHeaderName: name})
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
	// Early validation - get event name from header
	eventName := header[eventHeaderName]
	if eventName == "" {
		eq.service.Log(ctx).Error("Missing event header in message")
		return errors.New("missing event header")
	}

	// Get event handler from registry with proper error handling
	eventHandler, ok := eq.service.eventRegistry[eventName]
	if !ok {
		eq.service.Log(ctx).WithField("event", eventName).Error("Event not found in registry")
		return errors.New("event not found in registry: " + eventName)
	}

	// Get payload type template for efficient processing
	payloadTemplate := eventHandler.PayloadType()
	var processedPayload any

	// Optimize payload processing based on type with minimal allocations
	switch v := payloadTemplate.(type) {
	case []byte:
		// Direct byte slice - no allocation needed
		processedPayload = payload

	case json.RawMessage:
		// Direct raw message - no allocation needed
		processedPayload = json.RawMessage(payload)

	case string:
		// Convert to string and return pointer to match expected type
		processedPayload = string(payload)

	default:
		// Handle protobuf messages efficiently
		if protoMsg, ok0 := v.(proto.Message); ok0 {
			// Clone the prototype to avoid modifying the template
			clonedMsg := proto.Clone(protoMsg)
			if err := proto.Unmarshal(payload, clonedMsg); err != nil {
				eq.service.Log(ctx).WithError(err).WithField("event", eventName).Error("Failed to unmarshal protobuf payload")
				return err
			}
			processedPayload = clonedMsg
		} else {
			// Handle JSON unmarshaling with proper error context
			if err := json.Unmarshal(payload, &v); err != nil {
				eq.service.Log(ctx).WithError(err).WithField("event", eventName).Error("Failed to unmarshal JSON payload")
				return err
			}
			processedPayload = v
		}
	}

	// Validate payload with proper error context
	if err := eventHandler.Validate(ctx, processedPayload); err != nil {
		eq.service.Log(ctx).WithError(err).WithField("event", eventName).Error("Event payload validation failed")
		return err
	}

	// Execute event with proper error context
	if err := eventHandler.Execute(ctx, processedPayload); err != nil {
		eq.service.Log(ctx).WithError(err).WithField("event", eventName).Error("Event execution failed")
		return err
	}

	return nil
}
