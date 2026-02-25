# Logging

Frame uses `github.com/pitabwire/util` for structured logging. Logging is configured via `WithLogger` and `ConfigurationLogLevel`.

## Basic Usage

```go
_, svc := frame.NewService(frame.WithName("api"))

svc.Log(ctx).Info("service started")
svc.SLog(ctx).Info("structured", "key", "value")
```

## Configuration

Configure with environment variables or a custom config implementing:

- `LoggingLevel()`
- `LoggingFormat()`
- `LoggingTimeFormat()`
- `LoggingShowStackTrace()`
- `LoggingColored()`

## Telemetry Integration

When telemetry is enabled, Frame attaches an OpenTelemetry log handler. This allows trace correlation in supported backends.

## Best Practices

- Use `svc.Log(ctx)` for context-aware logging.
- Avoid logging sensitive data (tokens, secrets, PII).
- Keep logs structured for queryability.
