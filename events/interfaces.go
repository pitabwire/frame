package events

import (
	"context"

	"github.com/pitabwire/frame/queue"
)

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

type Manager interface {
	Add(eventI EventI)
	Get(name string) (EventI, error)
	Emit(ctx context.Context, name string, payload any) error
	Handler() queue.SubscribeWorker
}
