package telemetry

import (
	"context"
	"log/slog"
	"os"
	"runtime"

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

	"github.com/pitabwire/frame/config"
)

type Manager interface {
	Init(ctx context.Context) error
	Disabled() bool
	LogHandler() slog.Handler
}

type manager struct {
	serviceName        string
	serviceVersion     string
	serviceEnvironment string

	cfg config.ConfigurationTelemetry

	disableTracing bool

	traceTextMap      propagation.TextMapPropagator
	traceExporter     sdktrace.SpanExporter
	traceSampler      sdktrace.Sampler
	metricsReader     sdkmetrics.Reader
	traceLogsExporter sdklogs.Exporter

	logHandler slog.Handler
}

func (m *manager) LogHandler() slog.Handler {
	return m.logHandler
}

func (m *manager) Disabled() bool {
	return m.disableTracing
}

// NewManager creates a new telemetry setup manager.
func NewManager(ctx context.Context, cfg config.ConfigurationTelemetry, opts ...Option) Manager {
	m := &manager{
		cfg: cfg,
	}

	for _, opt := range opts {
		opt(ctx, m)
	}

	return m
}

func (m *manager) Init(ctx context.Context) error {
	if m.Disabled() {
		return nil
	}

	res, err := m.setupResource()
	if err != nil {
		return err
	}

	m.setupTextMapPropagator()
	m.setupTraceSampler()

	if err = m.setupTraceExporter(ctx); err != nil {
		return err
	}

	if err = m.setupMetricsReader(ctx); err != nil {
		return err
	}

	if err = m.setupLogsExporter(ctx); err != nil {
		return err
	}

	return m.setupProviders(ctx, res)
}

// setupResource creates and returns the OpenTelemetry resource.
func (m *manager) setupResource() (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(m.serviceName),
		semconv.ServiceVersion(m.serviceVersion),
		semconv.ServiceNamespace(m.serviceEnvironment),
		semconv.DeploymentEnvironmentName(m.serviceEnvironment),
		semconv.ProcessPID(os.Getpid()),
		semconv.ProcessRuntimeName("go"),
		semconv.ProcessRuntimeVersion(runtime.Version()),
	}

	return resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL, attrs...))
}

// setupTextMapPropagator initializes the text map propagator if not already set.
func (m *manager) setupTextMapPropagator() {
	if m.traceTextMap == nil {
		m.traceTextMap = autoprop.NewTextMapPropagator()
	}
}

// setupTraceSampler initializes the trace sampler if not already set.
func (m *manager) setupTraceSampler() {
	if m.traceSampler == nil {
		traceIDRatio := 1.0

		if m.cfg != nil {
			traceIDRatio = m.cfg.SamplingRatio()
		}

		m.traceSampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(traceIDRatio))
	}
}

// setupTraceExporter initializes the trace exporter if not already set.
func (m *manager) setupTraceExporter(ctx context.Context) error {
	if m.traceExporter == nil {
		if os.Getenv("OTEL_TRACES_EXPORTER") == "" {
			_ = os.Setenv("OTEL_TRACES_EXPORTER", "none")
		}
		var err error
		m.traceExporter, err = autoexport.NewSpanExporter(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// setupMetricsReader initializes the metrics reader if not already set.
func (m *manager) setupMetricsReader(ctx context.Context) error {
	if m.metricsReader == nil {
		if os.Getenv("OTEL_METRICS_EXPORTER") == "" {
			_ = os.Setenv("OTEL_METRICS_EXPORTER", "none")
		}
		var err error
		m.metricsReader, err = autoexport.NewMetricReader(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// setupLogsExporter initializes the logs exporter if not already set.
func (m *manager) setupLogsExporter(ctx context.Context) error {
	if m.traceLogsExporter == nil {
		if os.Getenv("OTEL_LOGS_EXPORTER") == "" {
			_ = os.Setenv("OTEL_LOGS_EXPORTER", "none")
		}
		var err error
		m.traceLogsExporter, err = autoexport.NewLogExporter(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// setupProviders initializes the OpenTelemetry providers and logger.
func (m *manager) setupProviders(_ context.Context, res *resource.Resource) error {
	otel.SetTextMapPropagator(m.traceTextMap)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(m.traceSampler),
		sdktrace.WithBatcher(m.traceExporter),
		sdktrace.WithResource(res))

	otel.SetTracerProvider(tp)

	mp := sdkmetrics.NewMeterProvider(
		sdkmetrics.WithReader(m.metricsReader),
		sdkmetrics.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	logsProcessor := sdklogs.NewBatchProcessor(m.traceLogsExporter)
	lp := sdklogs.NewLoggerProvider(
		sdklogs.WithResource(res),
		sdklogs.WithProcessor(logsProcessor),
	)
	global.SetLoggerProvider(lp)

	m.logHandler = otelslog.NewHandler("",
		otelslog.WithSource(true),
		otelslog.WithLoggerProvider(lp),
		otelslog.WithAttributes(res.Attributes()...))

	return nil
}
