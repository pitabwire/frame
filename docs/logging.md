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

## Credential redaction

Frame never logs credentials:

- Auth interceptors log a `token_fingerprint` (the first 8 hex characters
  of the token's SHA-256) instead of the token or Authorization header.
- Queue, cache and datastore URLs are logged and returned in errors with
  passwords and sensitive query parameters replaced by `xxxxx`. Use
  `data.RedactURL` / `DSN.Redacted()` for your own log fields, and
  `DSN.ScrubError(err)` for errors that may echo a connection string.
  `errors.As` / `errors.Unwrap` on a scrubbed error return the original,
  unscrubbed error, so log the scrubbed value.
- When body logging is enabled, credential fields (passwords, client
  secrets and assertions, OAuth tokens) in JSON and form bodies are masked.
- HTTP client spans have credential query parameters removed from
  `url.full`.

## Best Practices

- Use `svc.Log(ctx)` for context-aware logging.
- Avoid logging sensitive data (tokens, secrets, PII).
- Keep logs structured for queryability.
