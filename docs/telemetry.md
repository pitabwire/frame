# Telemetry (OpenTelemetry)

Frame provides first-class OpenTelemetry integration. Telemetry is initialized automatically by `WithTelemetry` (applied by default in `NewService`).

## What Frame Configures

- Resource attributes (service name, version, environment, build info).
- Tracing: sampler, exporter, propagators.
- Metrics: metric reader.
- Logs: OTel log exporter and slog bridge.

## Enable or Disable

Use configuration:

- `OPENTELEMETRY_DISABLE=true` to disable.
- `OPENTELEMETRY_TRACE_ID_RATIO` to control sampling.

## Custom Telemetry Options

```go
frame.WithTelemetry(
    telemetry.WithServiceName("orders"),
    telemetry.WithServiceVersion("1.2.3"),
)
```

Available options:

- `WithDisableTracing()`
- `WithServiceName(name string)`
- `WithServiceVersion(version string)`
- `WithServiceEnvironment(env string)`
- `WithPropagationTextMap(carrier propagation.TextMapPropagator)`
- `WithTraceExporter(exporter sdktrace.SpanExporter)`
- `WithTraceSampler(sampler sdktrace.Sampler)`
- `WithMetricsReader(reader sdkmetrics.Reader)`
- `WithTraceLogsExporter(exporter sdklogs.Exporter)`

## Tracing in Code

```go
tr := otel.Tracer("orders")
ctx, span := tr.Start(ctx, "checkout")
defer span.End()
```

## Best Practices

- Use span names that reflect business actions.
- Propagate context across goroutines.
- Keep sampling proportional to traffic.
