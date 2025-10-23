package frame

import (
	"context"
	"os"
	"runtime"

	"github.com/pitabwire/util"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklogs "go.opentelemetry.io/otel/sdk/log"
	sdkmetrics "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"

	config2 "github.com/pitabwire/frame/config"
)

type Tracer interface {
	Start(ctx context.Context, methodName string, options ...trace.SpanStartOption) (context.Context, trace.Span)
	End(ctx context.Context, span trace.Span, err error, options ...trace.SpanEndOption)
}

func (s *Service) initTracer(ctx context.Context) error {
	if s.disableTracing {
		return nil
	}

	res, err := s.setupResource()
	if err != nil {
		return err
	}

	s.setupTextMapPropagator()
	s.setupTraceSampler()

	if err = s.setupTraceExporter(ctx); err != nil {
		return err
	}

	if err = s.setupMetricsReader(ctx); err != nil {
		return err
	}

	if err = s.setupLogsExporter(ctx); err != nil {
		return err
	}

	return s.setupProviders(ctx, res)
}

// setupResource creates and returns the OpenTelemetry resource.
func (s *Service) setupResource() (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(s.Name()),
		semconv.ServiceVersion(s.Version()),
		semconv.ServiceNamespace(s.Environment()),
		semconv.DeploymentEnvironmentName(s.Environment()),
		semconv.ProcessPID(os.Getpid()),
		semconv.ProcessRuntimeName("go"),
		semconv.ProcessRuntimeVersion(runtime.Version()),
	}

	return resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL, attrs...))
}

// setupTextMapPropagator initializes the text map propagator if not already set.
func (s *Service) setupTextMapPropagator() {
	if s.traceTextMap == nil {
		s.traceTextMap = autoprop.NewTextMapPropagator()
	}
}

// setupTraceSampler initializes the trace sampler if not already set.
func (s *Service) setupTraceSampler() {
	if s.traceSampler == nil {
		traceIDRatio := 1.0

		config, ok := s.Config().(config2.ConfigurationTelemetry)
		if ok {
			traceIDRatio = config.SamplingRatio()
		}

		s.traceSampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(traceIDRatio))
	}
}

// setupTraceExporter initializes the trace exporter if not already set.
func (s *Service) setupTraceExporter(ctx context.Context) error {
	if s.traceExporter == nil {
		if os.Getenv("OTEL_TRACES_EXPORTER") == "" {
			_ = os.Setenv("OTEL_TRACES_EXPORTER", "none")
		}
		var err error
		s.traceExporter, err = autoexport.NewSpanExporter(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// setupMetricsReader initializes the metrics reader if not already set.
func (s *Service) setupMetricsReader(ctx context.Context) error {
	if s.metricsReader == nil {
		if os.Getenv("OTEL_METRICS_EXPORTER") == "" {
			_ = os.Setenv("OTEL_METRICS_EXPORTER", "none")
		}
		var err error
		s.metricsReader, err = autoexport.NewMetricReader(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// setupLogsExporter initializes the logs exporter if not already set.
func (s *Service) setupLogsExporter(ctx context.Context) error {
	if s.traceLogsExporter == nil {
		if os.Getenv("OTEL_LOGS_EXPORTER") == "" {
			_ = os.Setenv("OTEL_LOGS_EXPORTER", "none")
		}
		var err error
		s.traceLogsExporter, err = autoexport.NewLogExporter(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// setupProviders initializes the OpenTelemetry providers and logger.
func (s *Service) setupProviders(ctx context.Context, res *resource.Resource) error {
	otel.SetTextMapPropagator(s.traceTextMap)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(s.traceSampler),
		sdktrace.WithBatcher(s.traceExporter),
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

	logHandler := otelslog.NewHandler("",
		otelslog.WithSource(true),
		otelslog.WithLoggerProvider(lp),
		otelslog.WithAttributes(res.Attributes()...))
	log := util.NewLogger(ctx, util.WithLogHandler(logHandler))
	s.logger = log.WithField("service", s.Name())

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
