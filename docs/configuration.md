# Configuration

Frame uses `config.ConfigurationDefault` for environment-based configuration. You can provide your own config type as long as it implements the relevant interfaces.

## Load Configuration

```go
cfg, err := config.FromEnv[config.ConfigurationDefault]()
if err != nil {
    panic(err)
}

_, svc := frame.NewService(frame.WithConfig(&cfg))
```

To load OIDC metadata automatically:

```go
cfg, err := config.LoadWithOIDC[config.ConfigurationDefault](context.Background())
```

## Configuration Interfaces

Frame reads configuration through narrow interfaces. Implement only what you need:

- `ConfigurationService`
- `ConfigurationSecurity`
- `ConfigurationLogLevel`
- `ConfigurationTraceRequests`
- `ConfigurationProfiler`
- `ConfigurationPorts`
- `ConfigurationTelemetry`
- `ConfigurationWorkerPool`
- `ConfigurationOAUTH2`
- `ConfigurationJWTVerification`
- `ConfigurationAuthorization`
- `ConfigurationDatabase`
- `ConfigurationDatabaseTracing`
- `ConfigurationEvents`

## Environment Variables (ConfigurationDefault)

### Service and Runtime

| Env | Default | Description |
| --- | --- | --- |
| `SERVICE_NAME` | empty | Service name.
| `SERVICE_ENVIRONMENT` | empty | Environment name, used by telemetry.
| `SERVICE_VERSION` | empty | Service version.
| `RUN_SERVICE_SECURELY` | `true` | Enables security-sensitive defaults.
| `PORT` | `:7000` | Generic server port (fallback).
| `HTTP_PORT` | `:8080` | HTTP server port.
| `GRPC_PORT` | `:50051` | gRPC server port.

### Logging

| Env | Default | Description |
| --- | --- | --- |
| `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`).
| `LOG_FORMAT` | `info` | Log format (for util logger).
| `LOG_TIME_FORMAT` | `2006-01-02T15:04:05Z07:00` | Time format.
| `LOG_COLORED` | `true` | Enable ANSI color.
| `LOG_SHOW_STACK_TRACE` | `false` | Emit stack traces.

### Request Tracing

| Env | Default | Description |
| --- | --- | --- |
| `TRACE_REQUESTS` | `false` | Enable HTTP request tracing.
| `TRACE_REQUESTS_LOG_BODY` | `false` | Log request bodies.

### Profiler

| Env | Default | Description |
| --- | --- | --- |
| `PROFILER_ENABLE` | `false` | Enable pprof server.
| `PROFILER_PORT` | `:6060` | pprof address.

### OpenTelemetry

| Env | Default | Description |
| --- | --- | --- |
| `OPENTELEMETRY_DISABLE` | `false` | Disable OTel.
| `OPENTELEMETRY_TRACE_ID_RATIO` | `0.1` | Trace sampling ratio.

### Worker Pool

| Env | Default | Description |
| --- | --- | --- |
| `WORKER_POOL_CPU_FACTOR_FOR_WORKER_COUNT` | `10` | CPU multiplier for worker count.
| `WORKER_POOL_CAPACITY` | `100` | Queue capacity.
| `WORKER_POOL_COUNT` | `100` | Worker pool size.
| `WORKER_POOL_EXPIRY_DURATION` | `1s` | Worker idle expiry.

### TLS

| Env | Default | Description |
| --- | --- | --- |
| `TLS_CERTIFICATE_PATH` | empty | TLS certificate path.
| `TLS_CERTIFICATE_KEY_PATH` | empty | TLS key path.

### OAuth2 / OIDC

| Env | Default | Description |
| --- | --- | --- |
| `OAUTH2_SERVICE_URI` | empty | Base URL for OIDC provider.
| `OAUTH2_SERVICE_ADMIN_URI` | empty | Admin endpoint (if supported).
| `OAUTH2_WELL_KNOWN_OIDC_PATH` | `.well-known/openid-configuration` | OIDC discovery path.
| `OAUTH2_SERVICE_AUDIENCE` | empty | Expected audience values.
| `OAUTH2_SERVICE_CLIENT_ID` | empty | Client ID.
| `OAUTH2_SERVICE_CLIENT_SECRET` | empty | Client secret.
| `OAUTH2_WELL_KNOWN_JWK_DATA` | empty | Pre-fetched JWKS JSON.
| `OAUTH2_JWT_VERIFY_AUDIENCE` | empty | JWT audience verification.
| `OAUTH2_JWT_VERIFY_ISSUER` | empty | JWT issuer verification.

### Authorization

| Env | Default | Description |
| --- | --- | --- |
| `AUTHORIZATION_SERVICE_READ_URI` | empty | Read API for authorization service.
| `AUTHORIZATION_SERVICE_WRITE_URI` | empty | Write API for authorization service.

### Datastore

| Env | Default | Description |
| --- | --- | --- |
| `DATABASE_URL` | empty | Primary database URL(s).
| `REPLICA_DATABASE_URL` | empty | Replica database URL(s).
| `DO_MIGRATION` | `false` | Run migrations on startup.
| `MIGRATION_PATH` | `./migrations/0001` | Migration path.
| `SKIP_DEFAULT_TRANSACTION` | `true` | GORM transaction default.
| `PREFER_SIMPLE_PROTOCOL` | `true` | PG simple protocol.
| `DATABASE_MAX_IDLE_CONNECTIONS` | `2` | Max idle conns.
| `DATABASE_MAX_OPEN_CONNECTIONS` | `5` | Max open conns.
| `DATABASE_MAX_CONNECTION_LIFE_TIME_IN_SECONDS` | `300` | Conn max lifetime.
| `DATABASE_LOG_QUERIES` | `false` | Enable query logging.
| `DATABASE_SLOW_QUERY_THRESHOLD` | `200ms` | Slow query threshold.

### Events

| Env | Default | Description |
| --- | --- | --- |
| `EVENTS_QUEUE_NAME` | `frame.events.internal_._queue` | Internal event queue name.
| `EVENTS_QUEUE_URL` | `mem://frame.events.internal_._queue` | Internal event queue URL.

## YAML Example

```yaml
service_name: orders
service_environment: production
http_server_port: ":8080"
log_level: info
opentelemetry_disable: false
worker_pool_capacity: 200
```

## Custom Configuration

You can provide any config struct; implement only the interfaces required by the features you use:

```go
type MyConfig struct {
    Name string `env:"SERVICE_NAME"`
}

func (c *MyConfig) Name() string { return c.Name }
```

Then:

```go
cfg := &MyConfig{}
_ = config.FillEnv(cfg)
_, svc := frame.NewService(frame.WithConfig(cfg))
```
