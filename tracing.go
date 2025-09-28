package frame

import (
	"context"
	"os"

	"github.com/pitabwire/util"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklogs "go.opentelemetry.io/otel/sdk/log"
	sdkmetrics "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

func (s *Service) initTracer(ctx context.Context) error {
	if s.disableTracing {
		return nil
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(s.name),
			semconv.ServiceVersionKey.String(s.version),
			semconv.DeploymentEnvironmentNameKey.String(s.environment),
		),
	)

	if err != nil {
		return err
	}

	if s.traceTextMap == nil {
		s.traceTextMap = autoprop.NewTextMapPropagator()
	}

	if s.traceSampler == nil {
		s.traceSampler = sdktrace.AlwaysSample()
	}

	if s.traceExporter == nil {
		if os.Getenv("OTEL_TRACES_EXPORTER") == "" {
			_ = os.Setenv("OTEL_TRACES_EXPORTER", "none")
		}
		s.traceExporter, err = autoexport.NewSpanExporter(ctx)
		if err != nil {
			return err
		}
	}

	if s.metricsReader == nil {
		if os.Getenv("OTEL_METRICS_EXPORTER") == "" {
			_ = os.Setenv("OTEL_METRICS_EXPORTER", "none")
		}
		s.metricsReader, err = autoexport.NewMetricReader(ctx)
		if err != nil {
			return err
		}
	}

	if s.traceLogsExporter == nil {

		if os.Getenv("OTEL_LOGS_EXPORTER") == "" {
			_ = os.Setenv("OTEL_LOGS_EXPORTER", "none")
		}
		s.traceLogsExporter, err = autoexport.NewLogExporter(ctx)
		if err != nil {
			return err
		}
	}

	otel.SetTextMapPropagator(s.traceTextMap)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(s.traceSampler),
		sdktrace.WithSyncer(s.traceExporter),
		sdktrace.WithResource(res))

	otel.SetTracerProvider(tp)

	mp := sdkmetrics.NewMeterProvider(
		sdkmetrics.WithReader(s.metricsReader),
		sdkmetrics.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	logsProcessor := sdklogs.NewBatchProcessor(s.traceLogsExporter)
	lp := sdklogs.NewLoggerProvider(
		sdklogs.WithResource(res),
		sdklogs.WithProcessor(logsProcessor),
	)
	global.SetLoggerProvider(lp)

	logHandler := otelslog.NewHandler("", otelslog.WithSource(true),
		otelslog.WithLoggerProvider(lp), otelslog.WithAttributes(res.Attributes()...))
	log := util.NewLogger(ctx, util.WithLogHandler(logHandler))
	log.WithField("service", s.Name())
	s.logger = log

	return nil
}

// WithDisableTracing disable tracing for the service.
func WithDisableTracing() Option {
	return func(_ context.Context, s *Service) {
		s.disableTracing = true
	}
}

// WithPropagationTextMap specifies the trace baggage carrier exporter to use.
func WithPropagationTextMap(carrier propagation.TextMapPropagator) Option {
	return func(_ context.Context, s *Service) {
		s.traceTextMap = carrier
	}
}

// WithTraceExporter specifies the trace exporter to use.
func WithTraceExporter(exporter sdktrace.SpanExporter) Option {
	return func(_ context.Context, s *Service) {
		s.traceExporter = exporter
	}
}

// WithTraceSampler specifies the trace sampler to use.
func WithTraceSampler(sampler sdktrace.Sampler) Option {
	return func(_ context.Context, s *Service) {
		s.traceSampler = sampler
	}
}

// WithMetricsReader specifies the metrics reader for the service.
func WithMetricsReader(reader sdkmetrics.Reader) Option {
	return func(_ context.Context, s *Service) {
		s.metricsReader = reader
	}
}

// WithTraceLogsExporter specifies the trace logs exporter for the service.
func WithTraceLogsExporter(exporter sdklogs.Exporter) Option {
	return func(_ context.Context, s *Service) {
		s.traceLogsExporter = exporter
	}
}

// noopSpanExporter is a minimal noop implementation of sdktrace.SpanExporter
type noopSpanExporter struct{}

func (n *noopSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (n *noopSpanExporter) Shutdown(ctx context.Context) error {
	return nil
}

// noopLogExporter is a minimal noop implementation of sdklogs.Exporter
type noopLogExporter struct{}

func (n *noopLogExporter) Export(ctx context.Context, records []sdklogs.Record) error {
	return nil
}

func (n *noopLogExporter) Shutdown(ctx context.Context) error {
	return nil
}

func (n *noopLogExporter) ForceFlush(ctx context.Context) error {
	return nil
}
