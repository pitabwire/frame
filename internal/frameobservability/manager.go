package frameobservability

import (
	"context"
	"log/slog"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/pitabwire/util"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	sdklogs "go.opentelemetry.io/otel/sdk/log"
	sdkmetrics "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.34.0"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"runtime/debug"
)

// Manager implements the ObservabilityManager interface
type Manager struct {
	config            TracingConfig
	loggingConfig     LoggingConfig
	options           ObservabilityOptions
	logger            *util.LogEntry
	tracerProvider    *sdktrace.TracerProvider
	meterProvider     *sdkmetrics.MeterProvider
	loggerProvider    *sdklogs.LoggerProvider
}

// NewManager creates a new observability manager
func NewManager(config TracingConfig, loggingConfig LoggingConfig, options ObservabilityOptions) *Manager {
	return &Manager{
		config:        config,
		loggingConfig: loggingConfig,
		options:       options,
	}
}

// InitTracer initializes the OpenTelemetry tracing, metrics, and logging
func (m *Manager) InitTracer(ctx context.Context) error {
	if !m.options.EnableTracing {
		// Initialize basic logger even if tracing is disabled
		return m.initBasicLogger(ctx)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(m.config.ServiceName()),
			semconv.ServiceVersionKey.String(m.config.ServiceVersion()),
			semconv.DeploymentEnvironmentNameKey.String(m.config.Environment()),
		),
	)
	if err != nil {
		return err
	}

	// Initialize propagator
	if m.options.TraceTextMap == nil {
		m.options.TraceTextMap = autoprop.NewTextMapPropagator()
	}

	// Initialize sampler
	if m.options.TraceSampler == nil {
		m.options.TraceSampler = sdktrace.AlwaysSample()
	}

	// Initialize trace exporter
	if m.options.TraceExporter == nil {
		m.options.TraceExporter, err = otlptracegrpc.New(ctx)
		if err != nil {
			return err
		}
	}

	// Initialize metrics reader
	if m.options.MetricsReader == nil {
		metricsExporter, err0 := otlpmetricgrpc.New(ctx)
		if err0 != nil {
			return err0
		}
		m.options.MetricsReader = sdkmetrics.NewPeriodicReader(metricsExporter)
	}

	// Initialize logs exporter
	if m.options.TraceLogsExporter == nil {
		m.options.TraceLogsExporter, err = otlploggrpc.New(ctx)
		if err != nil {
			return err
		}
	}

	// Set up OpenTelemetry providers
	otel.SetTextMapPropagator(m.options.TraceTextMap)

	m.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithSampler(m.options.TraceSampler),
		sdktrace.WithSyncer(m.options.TraceExporter),
		sdktrace.WithResource(res))
	otel.SetTracerProvider(m.tracerProvider)

	m.meterProvider = sdkmetrics.NewMeterProvider(
		sdkmetrics.WithReader(m.options.MetricsReader),
		sdkmetrics.WithResource(res),
	)
	otel.SetMeterProvider(m.meterProvider)

	logsProcessor := sdklogs.NewBatchProcessor(m.options.TraceLogsExporter)
	m.loggerProvider = sdklogs.NewLoggerProvider(
		sdklogs.WithResource(res),
		sdklogs.WithProcessor(logsProcessor),
	)
	global.SetLoggerProvider(m.loggerProvider)

	// Initialize logger with OpenTelemetry integration
	logHandler := otelslog.NewHandler("", otelslog.WithSource(true),
		otelslog.WithLoggerProvider(m.loggerProvider), otelslog.WithAttributes(res.Attributes()...))
	log := util.NewLogger(ctx, util.WithLogHandler(logHandler))
	log.WithField("service", m.config.ServiceName())
	m.logger = log

	return nil
}

// initBasicLogger initializes a basic logger without OpenTelemetry integration
func (m *Manager) initBasicLogger(ctx context.Context) error {
	var opts []util.Option

	if m.loggingConfig != nil {
		logLevel, err := util.ParseLevel(m.loggingConfig.LoggingLevel())
		if err == nil {
			opts = append(opts, util.WithLogLevel(logLevel))
		}
		opts = append(opts,
			util.WithLogTimeFormat(m.loggingConfig.LoggingTimeFormat()),
			util.WithLogNoColor(!m.loggingConfig.LoggingColored()),
			util.WithLogStackTrace())
	}

	log := util.NewLogger(ctx, opts...)
	log.WithField("service", m.config.ServiceName())
	m.logger = log

	return nil
}

// Log returns a logger instance with context
func (m *Manager) Log(ctx context.Context) *util.LogEntry {
	if m.logger == nil {
		// Fallback to basic logger if not initialized
		return util.NewLogger(ctx).WithContext(ctx)
	}
	return m.logger.WithContext(ctx)
}

// SLog returns a structured logger instance with context
func (m *Manager) SLog(ctx context.Context) *slog.Logger {
	return m.Log(ctx).SLog()
}

// Shutdown gracefully shuts down all observability components
func (m *Manager) Shutdown(ctx context.Context) error {
	var lastErr error

	if m.tracerProvider != nil {
		if err := m.tracerProvider.Shutdown(ctx); err != nil {
			lastErr = err
		}
	}

	if m.meterProvider != nil {
		if err := m.meterProvider.Shutdown(ctx); err != nil {
			lastErr = err
		}
	}

	if m.loggerProvider != nil {
		if err := m.loggerProvider.Shutdown(ctx); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// GetLoggingOptions returns gRPC logging options for middleware
func GetLoggingOptions() []logging.Option {
	return []logging.Option{
		logging.WithLevels(func(code codes.Code) logging.Level {
			switch code {
			case codes.OK, codes.AlreadyExists:
				return logging.LevelDebug
			case codes.NotFound, codes.Canceled, codes.InvalidArgument, codes.Unauthenticated:
				return logging.LevelInfo
			case codes.DeadlineExceeded,
				codes.PermissionDenied,
				codes.ResourceExhausted,
				codes.FailedPrecondition,
				codes.Aborted,
				codes.OutOfRange,
				codes.Unavailable:
				return logging.LevelWarn
			case codes.Unknown, codes.Unimplemented, codes.Internal, codes.DataLoss:
				return logging.LevelError
			default:
				return logging.LevelError
			}
		}),
		logging.WithLogOnEvents(logging.StartCall, logging.FinishCall, logging.PayloadReceived, logging.PayloadSent),
	}
}

// RecoveryHandlerFunc returns a gRPC recovery handler function
func RecoveryHandlerFunc(manager ObservabilityManager) func(ctx context.Context, p any) error {
	return func(ctx context.Context, p any) error {
		manager.Log(ctx).WithField("trigger", p).Error("recovered from panic %s", debug.Stack())
		return status.Errorf(codes.Internal, "Internal server error")
	}
}
