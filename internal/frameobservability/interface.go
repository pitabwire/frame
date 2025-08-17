package frameobservability

import (
	"context"
	"log/slog"

	"github.com/pitabwire/util"
	"go.opentelemetry.io/otel/propagation"
	sdklogs "go.opentelemetry.io/otel/sdk/log"
	sdkmetrics "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ObservabilityManager defines the interface for observability functionality
type ObservabilityManager interface {
	// InitTracer initializes the OpenTelemetry tracing, metrics, and logging
	InitTracer(ctx context.Context) error
	
	// Logger returns a logger instance with context
	Log(ctx context.Context) *util.LogEntry
	
	// SLog returns a structured logger instance with context
	SLog(ctx context.Context) *slog.Logger
	
	// Shutdown gracefully shuts down all observability components
	Shutdown(ctx context.Context) error
}

// TracingConfig defines configuration for tracing setup
type TracingConfig interface {
	ServiceName() string
	ServiceVersion() string
	Environment() string
	EnableTracing() bool
}

// LoggingConfig defines configuration for logging setup
type LoggingConfig interface {
	LoggingLevel() string
	LoggingTimeFormat() string
	LoggingColored() bool
}

// ObservabilityOptions contains all observability configuration options
type ObservabilityOptions struct {
	// Tracing options
	EnableTracing     bool
	TraceTextMap      propagation.TextMapPropagator
	TraceExporter     sdktrace.SpanExporter
	TraceSampler      sdktrace.Sampler
	
	// Metrics options
	MetricsReader     sdkmetrics.Reader
	
	// Logging options
	TraceLogsExporter sdklogs.Exporter
	Logger            *util.LogEntry
}
