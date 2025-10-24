package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
	sdklogs "go.opentelemetry.io/otel/sdk/log"
	sdkmetrics "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Option func(ctx context.Context, m *manager)

// WithDisableTracing disable tracing for the service.
func WithDisableTracing() Option {
	return func(_ context.Context, s *manager) {
		s.disableTracing = true
	}
}

// WithServiceName sets the service name for resource tagging.
func WithServiceName(name string) Option {
	return func(_ context.Context, s *manager) {
		s.serviceName = name
	}
}

// WithServiceVersion sets the service version for resource tagging.
func WithServiceVersion(version string) Option {
	return func(_ context.Context, s *manager) {
		s.serviceName = version
	}
}

// WithServiceEnvironment sets the service environment for resource tagging.
func WithServiceEnvironment(env string) Option {
	return func(_ context.Context, s *manager) {
		s.serviceName = env
	}
}

// WithPropagationTextMap specifies the trace baggage carrier exporter to use.
func WithPropagationTextMap(carrier propagation.TextMapPropagator) Option {
	return func(_ context.Context, s *manager) {
		s.traceTextMap = carrier
	}
}

// WithTraceExporter specifies the trace exporter to use.
func WithTraceExporter(exporter sdktrace.SpanExporter) Option {
	return func(_ context.Context, s *manager) {
		s.traceExporter = exporter
	}
}

// WithTraceSampler specifies the trace sampler to use.
func WithTraceSampler(sampler sdktrace.Sampler) Option {
	return func(_ context.Context, s *manager) {
		s.traceSampler = sampler
	}
}

// WithMetricsReader specifies the metrics reader for the service.
func WithMetricsReader(reader sdkmetrics.Reader) Option {
	return func(_ context.Context, s *manager) {
		s.metricsReader = reader
	}
}

// WithTraceLogsExporter specifies the trace logs exporter for the service.
func WithTraceLogsExporter(exporter sdklogs.Exporter) Option {
	return func(_ context.Context, s *manager) {
		s.traceLogsExporter = exporter
	}
}
