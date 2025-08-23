package frameobservability

import (
	"context"

	"github.com/pitabwire/frame/internal/common"
	"github.com/pitabwire/util"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklogs "go.opentelemetry.io/otel/sdk/log"
	sdkmetrics "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

func initTracer(ctx context.Context, s common.Service) error {
	// Get tracing configuration from ObservabilityModule
	obsModule := s.GetModule(common.ModuleTypeObservability)
	if obsModule == nil {
		return nil
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(s.Name()),
			semconv.ServiceVersionKey.String("unknown"), // Version not available from interface
			semconv.DeploymentEnvironmentNameKey.String("unknown"), // Environment not available from interface
		),
	)

	if err != nil {
		return err
	}

	// Since obsModule is interface{}, we'll use default configurations
	traceTextMap := autoprop.NewTextMapPropagator()
	traceSampler := sdktrace.AlwaysSample()
	
	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return err
	}

	metricsExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return err
	}
	metricsReader := sdkmetrics.NewPeriodicReader(metricsExporter)

	traceLogsExporter, err := otlploggrpc.New(ctx)
	if err != nil {
		return err
	}

	otel.SetTextMapPropagator(traceTextMap)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(traceSampler),
		sdktrace.WithSyncer(traceExporter),
		sdktrace.WithResource(res))

	otel.SetTracerProvider(tp)

	mp := sdkmetrics.NewMeterProvider(
		sdkmetrics.WithReader(metricsReader),
		sdkmetrics.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	logsProcessor := sdklogs.NewBatchProcessor(traceLogsExporter)
	lp := sdklogs.NewLoggerProvider(
		sdklogs.WithResource(res),
		sdklogs.WithProcessor(logsProcessor),
	)
	global.SetLoggerProvider(lp)

	logHandler := otelslog.NewHandler("", otelslog.WithSource(true),
		otelslog.WithLoggerProvider(lp), otelslog.WithAttributes(res.Attributes()...))
	log := util.NewLogger(ctx, util.WithLogHandler(logHandler))
	log.WithField("service", s.Name())
	// Note: Cannot assign to s.logger as Service is an interface

	return nil
}

// WithEnableTracing disable tracing for the service.
func WithEnableTracing() common.Option {
	return func(_ context.Context, s common.Service) {
		// Note: Module interface doesn't expose specific methods, so we use placeholder
		// This will be implemented by the actual ObservabilityModule
		newModule := common.NewObservabilityModule(
			nil, // ObservabilityManager
			true, // Enable tracing
			nil, // TraceTextMap
			nil, // TraceExporter
			nil, // TraceSampler
			nil, // MetricsReader
			nil, // TraceLogsExporter
		)
		s.RegisterModule(newModule)
	}
}

// WithPropagationTextMap specifies the trace baggage carrier exporter to use.
func WithPropagationTextMap(carrier propagation.TextMapPropagator) common.Option {
	return func(_ context.Context, s common.Service) {
		// Note: Module interface doesn't expose specific methods, so we use placeholder
		// This will be implemented by the actual ObservabilityModule
		newModule := common.NewObservabilityModule(
			nil, // ObservabilityManager
			false, // EnableTracing
			carrier, // Set trace text map
			nil, // TraceExporter
			nil, // TraceSampler
			nil, // MetricsReader
			nil, // TraceLogsExporter
		)
		s.RegisterModule(newModule)
	}
}

// WithTraceExporter specifies the trace exporter to use.
func WithTraceExporter(exporter sdktrace.SpanExporter) common.Option {
	return func(_ context.Context, s common.Service) {
		// Note: Module interface doesn't expose specific methods, so we use placeholder
		// This will be implemented by the actual ObservabilityModule
		newModule := common.NewObservabilityModule(
			nil, // ObservabilityManager
			false, // EnableTracing
			nil, // TraceTextMap
			exporter, // Set trace exporter
			nil, // TraceSampler
			nil, // MetricsReader
			nil, // TraceLogsExporter
		)
		s.RegisterModule(newModule)
	}
}

// WithTraceSampler specifies the trace sampler to use.
func WithTraceSampler(sampler sdktrace.Sampler) common.Option {
	return func(_ context.Context, s common.Service) {
		// Note: Module interface doesn't expose specific methods, so we use placeholder
		// This will be implemented by the actual ObservabilityModule
		newModule := common.NewObservabilityModule(
			nil, // ObservabilityManager
			false, // EnableTracing
			nil, // TraceTextMap
			nil, // TraceExporter
			sampler, // Set trace sampler
			nil, // MetricsReader
			nil, // TraceLogsExporter
		)
		s.RegisterModule(newModule)
	}
}

// WithMetricsReader specifies the metrics reader for the service.
func WithMetricsReader(reader sdkmetrics.Reader) common.Option {
	return func(_ context.Context, s common.Service) {
		// Note: Module interface doesn't expose specific methods, so we use placeholder
		// This will be implemented by the actual ObservabilityModule
		newModule := common.NewObservabilityModule(
			nil, // ObservabilityManager
			false, // EnableTracing
			nil, // TraceTextMap
			nil, // TraceExporter
			nil, // TraceSampler
			reader, // Set metrics reader
			nil, // TraceLogsExporter
		)
		s.RegisterModule(newModule)
	}
}

// WithTraceLogsExporter specifies the trace logs exporter for the service.
func WithTraceLogsExporter(exporter sdklogs.Exporter) common.Option {
	return func(_ context.Context, s common.Service) {
		// Note: Module interface doesn't expose specific methods, so we use placeholder
		// This will be implemented by the actual ObservabilityModule
		newModule := common.NewObservabilityModule(
			nil, // ObservabilityManager
			false, // EnableTracing
			nil, // TraceTextMap
			nil, // TraceExporter
			nil, // TraceSampler
			nil, // MetricsReader
			exporter, // Set trace logs exporter
		)
		s.RegisterModule(newModule)
	}
}
