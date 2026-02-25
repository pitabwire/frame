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
- `Run` starts HTTP/gRPC servers, background processing, and startup hooks.
- `Stop` executes cleanup and shuts down gracefully.

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
- `AddHealthCheck(checker Checker)`
- `GetStartupErrors() []error`

## Core Options

Service options are composable and can be applied at construction or later via `Service.Init`.

### Server and Runtime

- `WithHTTPHandler(h http.Handler)`
- `WithHTTPMiddleware(mw ...func(http.Handler) http.Handler)`
- `WithGRPCServer(grpcServer *grpc.Server)`
- `WithGRPCPort(port string)`
- `WithEnableGRPCServerReflection()`
- `WithGRPCServerListener(listener net.Listener)`
- `WithDriver(driver ServerDriver)`

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

## Health Checks

Health checks are registered using `AddHealthCheck`, and are served on `/healthz` by default.

## Error Semantics

- Startup errors are collected via `AddStartupError` and returned when `Run` is called.
- `ErrorIsNotFound(err)` helps normalize "not found" checks across data, gRPC, and HTTP.

