# Plugins and Options (Frame Extension Model)

Frame's plugin system is the **options pattern**: a plugin is a reusable `frame.Option` that configures the service and registers lifecycle hooks.

This makes extensions composable, testable, and consistent across services.

## What Is a Frame Plugin?

A Frame plugin is:

- A `func(ctx context.Context, s *frame.Service)`
- Packaged as a helper `WithXxx(...) frame.Option`
- Able to register startup, shutdown, and health hooks

## Minimal Plugin Example

```go
func WithExamplePlugin() frame.Option {
    return func(ctx context.Context, s *frame.Service) {
        s.AddPreStartMethod(func(ctx context.Context, s *frame.Service) {
            s.Log(ctx).Info("example plugin starting")
        })

        s.AddCleanupMethod(func(ctx context.Context) {
            s.Log(ctx).Info("example plugin stopping")
        })
    }
}
```

## Lifecycle Hooks

| Hook | When It Runs | Typical Use |
| --- | --- | --- |
| `AddPublisherStartup` | Before subscribers | Declare topics, ensure queue exists |
| `AddSubscriberStartup` | After publishers | Register subscribers or workers |
| `AddPreStartMethod` | Before serving requests | Warm caches, load config, migrations |
| `AddCleanupMethod` | On shutdown | Close pools, flush metrics, stop goroutines |
| `AddHealthCheck` | Periodic health probe | DB, cache, external dependencies |

## How Options Compose

Options are evaluated in order, and multiple plugins can be combined:

```go
ctx, svc := frame.NewService(
    WithExamplePlugin(),
    frame.WithTelemetry(),
    frame.WithLogger(),
)
```

## Plugin Design Guidelines

- Keep options idempotent.
- Add cleanup for any goroutines or connections.
- Make configuration explicit (env or parameter).
- Prefer `AddPublisherStartup` and `AddSubscriberStartup` for queue-based plugins.
