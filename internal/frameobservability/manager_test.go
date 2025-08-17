package frameobservability

import (
	"context"
	"testing"
	"time"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testnats"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/stretchr/testify/suite"
	sdklogs "go.opentelemetry.io/otel/sdk/log"
	sdkmetrics "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type ObservabilityTestSuite struct {
	frametests.FrameBaseTestSuite
}

func TestObservabilityTestSuite(t *testing.T) {
	suite.Run(t, &ObservabilityTestSuite{
		FrameBaseTestSuite: frametests.FrameBaseTestSuite{
			InitResourceFunc: func(_ context.Context) []definition.TestResource {
				return []definition.TestResource{
					testpostgres.New(),
					testnats.New(),
				}
			},
		},
	})
}

// mockTracingConfig implements TracingConfig for testing
type mockTracingConfig struct {
	serviceName    string
	serviceVersion string
	environment    string
	enableTracing  bool
}

func (m *mockTracingConfig) ServiceName() string    { return m.serviceName }
func (m *mockTracingConfig) ServiceVersion() string { return m.serviceVersion }
func (m *mockTracingConfig) Environment() string    { return m.environment }
func (m *mockTracingConfig) EnableTracing() bool    { return m.enableTracing }

// mockLoggingConfig implements LoggingConfig for testing
type mockLoggingConfig struct {
	loggingLevel      string
	loggingTimeFormat string
	loggingColored    bool
}

func (m *mockLoggingConfig) LoggingLevel() string      { return m.loggingLevel }
func (m *mockLoggingConfig) LoggingTimeFormat() string { return m.loggingTimeFormat }
func (m *mockLoggingConfig) LoggingColored() bool      { return m.loggingColored }

// noopLogExporter implements sdklogs.Exporter for testing
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

func (s *ObservabilityTestSuite) TestObservabilityOperations() {
	depOptions := []*definition.DependancyOption{
		definition.NewDependancyOption("postgres_nats", "observability_test", s.Resources()),
	}

	frametests.WithTestDependancies(s.T(), depOptions, func(t *testing.T, depOpt *definition.DependancyOption) {
		ctx := context.Background()
		
		// Create a service with the test dependencies
		ctx, svc := frame.NewServiceWithContext(ctx, "observability-test",
			frame.WithDatastoreConnection(depOpt.Database(ctx)[0].GetDS(ctx).String(), false),
			frame.WithRegisterPublisher("events", "mem://test-events"),
		)
		defer svc.Stop(ctx)

	testCases := []struct {
		name           string
		tracingConfig  TracingConfig
		loggingConfig  LoggingConfig
		options        ObservabilityOptions
		enableTracing  bool
		expectError    bool
	}{
		{
			name: "BasicLoggingOnly",
			tracingConfig: &mockTracingConfig{
				serviceName:    "test-service",
				serviceVersion: "1.0.0",
				environment:    "test",
				enableTracing:  false,
			},
			loggingConfig: &mockLoggingConfig{
				loggingLevel:      "info",
				loggingTimeFormat: "2006-01-02T15:04:05Z07:00",
				loggingColored:    false,
			},
			options: ObservabilityOptions{
				EnableTracing: false,
			},
			enableTracing: false,
			expectError:   false,
		},
		{
			name: "TracingWithMockExporter",
			tracingConfig: &mockTracingConfig{
				serviceName:    "test-service",
				serviceVersion: "1.0.0",
				environment:    "test",
				enableTracing:  true,
			},
			loggingConfig: &mockLoggingConfig{
				loggingLevel:      "debug",
				loggingTimeFormat: "2006-01-02T15:04:05Z07:00",
				loggingColored:    true,
			},
			options: ObservabilityOptions{
				EnableTracing:      true,
				TraceExporter:      tracetest.NewInMemoryExporter(),
				MetricsReader:      sdkmetrics.NewManualReader(),
				TraceLogsExporter:  &noopLogExporter{},
			},
			enableTracing: true,
			expectError:   false,
		},
	}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Test manager creation
				manager := NewManager(tc.tracingConfig, tc.loggingConfig, tc.options)
				s.Require().NotNil(manager, "Should create manager successfully")

				// Test tracer initialization
				err := manager.InitTracer(ctx)
				if tc.expectError {
					s.Require().Error(err, "Should return error for invalid configuration")
					return
				}
				s.Require().NoError(err, "Should initialize tracer successfully")

				// Test logging functionality
				logger := manager.Log(ctx)
				s.Require().NotNil(logger, "Should return logger instance")

				slogger := manager.SLog(ctx)
				s.Require().NotNil(slogger, "Should return structured logger instance")

				// Test logger functionality
				logger.Info("Test log message")
				logger.WithField("test_field", "test_value").Debug("Test debug message")

				// Test shutdown
				err = manager.Shutdown(ctx)
				s.Require().NoError(err, "Should shutdown gracefully")
			})
		}
	})
}

func (s *ObservabilityTestSuite) TestLoggingOptions() {
	// Test gRPC logging options
	options := GetLoggingOptions()
	s.Require().NotEmpty(options, "Should return logging options")
}

func (s *ObservabilityTestSuite) TestRecoveryHandler() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a basic manager for testing
	config := &mockTracingConfig{
		serviceName:    "test-service",
		serviceVersion: "1.0.0",
		environment:    "test",
		enableTracing:  false,
	}
	loggingConfig := &mockLoggingConfig{
		loggingLevel:      "info",
		loggingTimeFormat: "2006-01-02T15:04:05Z07:00",
		loggingColored:    false,
	}
	options := ObservabilityOptions{EnableTracing: false}

	manager := NewManager(config, loggingConfig, options)
	err := manager.InitTracer(ctx)
	s.Require().NoError(err, "Should initialize manager")

	// Test recovery handler
	recoveryHandler := RecoveryHandlerFunc(manager)
	s.Require().NotNil(recoveryHandler, "Should return recovery handler")

	// Test recovery functionality
	err = recoveryHandler(ctx, "test panic")
	s.Require().Error(err, "Should return gRPC error")
	s.Require().Contains(err.Error(), "Internal server error", "Should contain error message")
}
