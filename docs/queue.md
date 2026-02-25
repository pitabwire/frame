# Queue (Go Cloud Pub/Sub)

Frame wraps Go Cloud Pub/Sub to provide queue publishing and subscription with a stable interface.

## Concepts

- `Publisher`: publish messages to a named queue.
- `Subscriber`: receive messages and dispatch to handlers.
- `Manager`: register and manage publishers/subscribers.

## Quick Start

```go
_, svc := frame.NewService(
    frame.WithRegisterPublisher("orders", "mem://orders"),
    frame.WithRegisterSubscriber("orders", "mem://orders", handler),
)

_ = svc.QueueManager().Publish(ctx, "orders", OrderCreated{ID: "123"})
```

## URL-Based Drivers

Frame uses Go Cloud URL schemes for pluggable queues:

- `mem://` in-memory
- `nats://` via `github.com/pitabwire/natspubsub`

Register driver packages with blank imports in your main package.

## Subscriber Handlers

```go
type handler struct{}
func (h handler) Handle(ctx context.Context, metadata map[string]string, message []byte) error {
    return nil
}
```

## Manager API

- `AddPublisher(ctx, ref, queueURL)`
- `AddSubscriber(ctx, ref, queueURL, handlers...)`
- `Publish(ctx, ref, payload, headers...)`
- `GetPublisher(ref)` / `GetSubscriber(ref)`
- `Init(ctx)` / `Close(ctx)`

## Metrics

Subscribers expose `SubscriberMetrics` to inspect idle time and processing time.

## Best Practices

- Register publishers before subscribers (Frame enforces ordering).
- Keep handler work short; offload to worker pool for heavy tasks.
- Use structured metadata headers for trace correlation.
