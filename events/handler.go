package events

import (
	"context"
	"errors"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/v2/internal"
)

const EventHeaderName = "frame._internal.event.header"

// ErrUnregisteredEvent is returned by the registry lookup when no
// handler is wired for an incoming event name. The queue handler
// translates this into either a Nack-with-error (strict mode, default)
// or a quiet Ack-and-skip (loose mode) depending on the manager's
// configuration. Exporting it lets callers detect the case rather
// than string-match on the error message.
var ErrUnregisteredEvent = errors.New("event not found in registry")

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
		// When the manager is in non-strict mode (the default for
		// shared-stream consumers that intentionally subscribe to a
		// catch-all subject and only care about a subset of events),
		// an unregistered event is acked-and-skipped here so the
		// stream cursor advances without the consumer needing a
		// per-topic Noop handler in application code. Strict mode
		// preserves the legacy "loud nack" behaviour for callers
		// that maintain a closed handler set.
		if !eq.manager.Strict() && errors.Is(err, ErrUnregisteredEvent) {
			util.Log(ctx).
				WithField("event", eventName).
				Debug("Event not in registry — acking (manager loose mode)")
			return nil
		}
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
