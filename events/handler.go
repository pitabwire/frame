package events

import (
	"context"
	"errors"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/internal"
)

const EventHeaderName = "frame._internal.event.header"

type eventQueueHandler struct {
	manager Manager
}

func (eq *eventQueueHandler) Handle(ctx context.Context, header map[string]string, payload []byte) error {
	// Early validation - get event name from header
	eventName := header[EventHeaderName]
	if eventName == "" {
		util.Log(ctx).Error("Missing event header in message")
		return errors.New("missing event header")
	}

	// Get event handler from registry with proper error handling
	eventHandler, err := eq.manager.Get(eventName)
	if err != nil {
		util.Log(ctx).WithError(err).WithField("event", eventName).Error("Event not found in registry")
		return err
	}

	// Get payload type template for efficient processing
	payloadTemplate := eventHandler.PayloadType()

	err = internal.Unmarshal(payload, payloadTemplate)
	if err != nil {
		util.Log(ctx).WithError(err).WithField("event", eventName).Error("Failed to unmarshal payload")
		return err
	}

	// Validate payload with proper error context
	err = eventHandler.Validate(ctx, payloadTemplate)
	if err != nil {
		util.Log(ctx).WithError(err).WithField("event", eventName).Error("Event payload validation failed")
		return err
	}

	// Execute event with proper error context
	err = eventHandler.Execute(ctx, payloadTemplate)
	if err != nil {
		util.Log(ctx).WithError(err).WithField("event", eventName).Error("Event execution failed")
		return err
	}

	return nil
}
