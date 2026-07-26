# Frame Service Lifecycle and API

The `frame.Service` is the core runtime. It owns configuration, managers, servers, and lifecycle hooks. Most features are enabled by passing `Option` functions to `frame.NewService`.

## Core Service Construction

```go
ctx, svc := frame.NewService(
    frame.WithName("orders"),
    frame.WithVersion("1.2.3"),
    frame.WithEnvironment("prod"),
)
```

You can also supply an explicit context:

```go
ctx := context.Background()
ctx, svc := frame.NewServiceWithContext(ctx, frame.WithName("orders"))
```

## Service Lifecycle

- `NewService` applies options and initializes core managers.
- `Run` starts the HTTP server, background processing, and startup hooks.
- `Stop` executes cleanup and shuts down gracefully.
- **Setup jobs** (migrate / permissions / bootstrap) use the abstract
  [`setup`](../setup) package and `Service.RunSetup` — see
  [SETUP_JOB.md](./SETUP_JOB.md). Do not put one-shot deploy work only in
  runtime PreStart.

### Startup Hooks (Ordering Matters)

Frame registers startup methods with strict ordering:

1. Publisher startup hooks
2. Subscriber startup hooks
3. Other startup hooks

This ensures in-memory queue topics exist before subscribers are started.

## Core API

### Constructors

- `NewService(opts ...Option) (context.Context, *Service)`
- `NewServiceWithContext(ctx context.Context, opts ...Option) (context.Context, *Service)`

### Service Methods

- `Name() string` / `WithName(name string)`
- `Version() string` / `WithVersion(version string)`
- `Environment() string` / `WithEnvironment(env string)`
- `Config() any` / `WithConfig(cfg any)`
- `Run(ctx context.Context, address string) error`
- `Stop(ctx context.Context)`
- `AddPreStartMethod(func(ctx context.Context, s *Service))`
- `AddPublisherStartup(func(ctx context.Context, s *Service))`
- `AddSubscriberStartup(func(ctx context.Context, s *Service))`
- `AddCleanupMethod(func(ctx context.Context))`
- `AddHealthCheck(checker Checker)` — readiness/dependency checks for `/readyz` and `/healthz`
- `AddLivenessCheck(checker Checker)` — optional process-level checks for `/livez`
- `GetStartupErrors() []error`

## Core Options

Service options are composable and can be applied at construction or later via `Service.Init`.

### Server and Runtime

- `WithHTTPHandler(h http.Handler)`
- `WithHTTPMiddleware(mw ...func(http.Handler) http.Handler)`
- `WithDriver(driver server.Driver)`

### Configuration and Logging

- `WithConfig(cfg any)`
- `WithLogger(opts ...util.Option)`

### Telemetry and Clients

- `WithTelemetry(opts ...telemetry.Option)`
- `WithHTTPClient(opts ...client.HTTPOption)`

### Datastore

- `WithDatastoreManager()`
- `WithDatastore(opts ...pool.Option)`
- `WithDatastoreConnection(dsn string, readOnly bool)`
- `WithDatastoreConnectionWithName(name, dsn string, readOnly bool, opts ...pool.Option)`

### Cache

- `WithCacheManager()`
- `WithCache(name string, raw cache.RawCache)`
- `WithInMemoryCache(name string)`

### Queue and Events

- `WithRegisterPublisher(reference, queueURL string)`
- `WithRegisterSubscriber(reference, queueURL string, handlers ...queue.SubscribeWorker)`
- `WithRegisterEvents(evt ...events.EventI)`

### Localization

- `WithTranslation(translationsFolder string, languages ...string)`

### Security

- `WithRegisterServerOauth2Client()`

### Worker Pool

- `WithBackgroundConsumer(func(ctx context.Context) error)`
- `WithWorkerPoolOptions(opts ...workerpool.Option)`

## Service Managers

- `DatastoreManager() datastore.Manager`
- `QueueManager() queue.Manager`
- `EventsManager() events.Manager`
- `CacheManager() cache.Manager`
- `TelemetryManager() telemetry.Manager`
- `SecurityManager() security.Manager`
- `LocalizationManager() localization.Manager`
- `WorkManager() workerpool.Manager`
- `HTTPClientManager() client.Manager`

## Health Checks (Kubernetes probes)

Frame always registers the three Kubernetes API health endpoints
([docs](https://kubernetes.io/docs/reference/using-api/health-checks/)):

| Path | Probe | Behaviour |
| --- | --- | --- |
| `/livez` | Liveness | Process is alive; restart only on non-recoverable faults. Does **not** check dependencies. Stays healthy during graceful shutdown. |
| `/readyz` | Readiness | Ready to accept traffic. Fails until startup completes, while terminating, or when any readiness checker fails. |
| `/healthz` | Deprecated | Same semantics as `/readyz`. Prefer `/livez` + `/readyz` for new deployments. |

```go
svc.AddHealthCheck(frame.CheckerFunc(func() error {
    return db.PingContext(ctx) // readiness only
}))

// Rare: process-level liveness (never external deps)
svc.AddLivenessCheck(frame.CheckerFunc(detectDeadlock))
```

Probe handlers return HTTP `200` when healthy and `503` when not. Machines
should rely on the status code; the JSON body is for operators.

Suggested Pod probe configuration:

```yaml
startupProbe:
  httpGet: { path: /readyz, port: 8080 }
livenessProbe:
  httpGet: { path: /livez, port: 8080 }
readinessProbe:
  httpGet: { path: /readyz, port: 8080 }
```

## Error Semantics

- Startup errors are collected via `AddStartupError` and returned when `Run` is called.
- `ErrorIsNotFound(err)` helps normalize "not found" checks across data, gRPC, and HTTP.
