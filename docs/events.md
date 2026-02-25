# Events (Internal Event Bus)

Frame includes an internal event registry and event queue that builds on the queue manager.

## Overview

- Define events using `events.EventI`.
- Register events via `frame.WithRegisterEvents`.
- Emit events through `events.Manager.Emit`.
- Events are dispatched via the internal event queue.

## Quick Start

```go
type UserCreated struct{}

func (UserCreated) Name() string { return "user.created" }
func (UserCreated) PayloadType() any { return UserPayload{} }
func (UserCreated) Validate(ctx context.Context, payload any) error { return nil }
func (UserCreated) Execute(ctx context.Context, payload any) error {
    return nil
}

_, svc := frame.NewService(
    frame.WithRegisterEvents(UserCreated{}),
)

_ = svc.EventsManager().Emit(ctx, "user.created", UserPayload{ID: "123"})
```

## How It Works

- `WithRegisterEvents` registers event definitions.
- `Service.setupEventsQueue` creates an internal publisher/subscriber pair for events.
- The event handler pulls messages from the queue, resolves the event by name, validates, and executes it.

## Configuration

The internal queue uses `ConfigurationEvents`:

- `EVENTS_QUEUE_NAME`
- `EVENTS_QUEUE_URL`

Defaults use `mem://` for in-memory events.

## Best Practices

- Keep event handlers idempotent.
- Avoid long-running work; delegate to worker pool.
- Use stable event names (`resource.action`).
