# Frame Architecture Overview

Frame is a fast, extensible Go framework built around a minimal core `Service` and a set of modular managers (datastore, queue, cache, events, telemetry, security, worker pool). The design emphasizes:

- Modular components that can be added or omitted per service.
- Convention-driven ergonomics (sensible defaults, minimal boilerplate).
- First-class plugin system via Go Cloud URL-based drivers.
- Strong runtime management with deterministic startup/shutdown.

## Mental Model

Frame bootstraps a `Service` that owns shared runtime state and managers. Options configure the service and register startup hooks. The service then starts HTTP/gRPC servers, background workers, and pluggable components.

```
ctx, svc := frame.NewService(
  frame.WithName("my-service"),
  frame.WithHTTPHandler(handler),
  // other options...
)
_ = svc.Run(ctx, ":8080")
```

```
Service
  -> Config (env, YAML, or custom)
  -> Telemetry (OTel)
  -> Logger (slog + telemetry handler)
  -> HTTP client (resilient transport)
  -> Worker pool
  -> Queue manager (pubsub)
  -> Events manager (internal event bus)
  -> Datastore manager (GORM + pool)
  -> Cache manager
  -> Security manager (authn/authz)
  -> Localization manager
```

## Runtime Lifecycle

```
NewService
  -> load configuration
  -> apply options (WithTelemetry, WithLogger, WithHTTPClient, custom)
  -> create managers (security, worker pool, queue, events)
  -> register startup hooks (publishers, subscribers, prestart)
  -> return service context

Run
  -> validate startup errors
  -> initialize queue manager
  -> start background consumer (if configured)
  -> start HTTP/gRPC server(s)
  -> start profiler (if enabled)
  -> execute startup hooks
  -> block until shutdown or error

Stop
  -> stop profiler
  -> cancel service context
  -> run cleanup hooks
```

## Extension Points (Plugin Architecture)

Frame uses Go Cloud to allow infrastructure to be configured by URL. Drivers are registered via blank imports and selected by URL scheme. This applies to:

- Pub/Sub (queue): `mem://`, `nats://`, etc.
- Cache: Redis, Valkey, JetStream KV, in-memory.
- Datastore: connection pools with GORM.

In practice, plugin extension looks like:

- Import driver packages in your main package.
- Provide a URL or DSN in config.
- Frame managers resolve the correct driver at runtime.

## Key Packages

- `frame`: core service, options, server lifecycle.
- `config`: configuration interfaces and env parsing.
- `datastore`: GORM pool + migrations.
- `queue`: Go Cloud pub/sub wrappers.
- `events`: event registry and event bus.
- `cache`: raw and typed cache, multi-backend.
- `telemetry`: OpenTelemetry setup.
- `security`: authentication, authorization, interceptors.
- `workerpool`: job execution and retry scheduler.
- `client`: resilient HTTP client with circuit breakers and retries.

## When to Extend vs When to Replace

Extend Frame when:
- You want standardized runtime, observability, and infrastructure wiring.
- You need a service that can be ported across cloud providers.
- You want consistent middleware, security, and lifecycle control.

Replace or bypass Frame when:
- The runtime lifecycle conflicts with your requirements.
- You need a custom server loop or non-HTTP primary entrypoint.
