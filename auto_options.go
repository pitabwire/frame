package frame

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/pitabwire/util"
	"github.com/pitabwire/frame/internal/common"
	"github.com/pitabwire/frame/internal/frameauth"
	"github.com/pitabwire/frame/internal/frameauthorization"
	"github.com/pitabwire/frame/internal/framedata"
	"github.com/pitabwire/frame/internal/frameobservability"
	"github.com/pitabwire/frame/internal/framequeue"
	"github.com/pitabwire/frame/internal/frameserver"
	"google.golang.org/grpc"
)

// authLoggerAdapter adapts util.LogEntry to frameauth.Logger interface
type authLoggerAdapter struct {
	logger *util.LogEntry
}

// serverManagerAdapter adapts common.ServerManager to frameserver.ServerManager interface
type serverManagerAdapter struct {
	commonServerManager common.ServerManager
}

func (s *serverManagerAdapter) GetServerStats() frameserver.ServerStats {
	commonStats := s.commonServerManager.GetServerStats()
	// Convert common.ServerStats to frameserver.ServerStats struct
	return frameserver.ServerStats{
		HTTPAddress:      s.GetHTTPAddress(),
		GRPCAddress:      s.GetGRPCAddress(),
		StartTime:        time.Now(), // TODO: track actual start time
		ActiveRequests:   commonStats.GetActiveConnections(),
		TotalRequests:    commonStats.GetTotalRequests(),
		ErrorCount:       0, // TODO: implement error count tracking
	}
}

func (s *serverManagerAdapter) GetGRPCAddress() string {
	// TODO: Implement proper GRPC address retrieval from common server manager
	return ":50051" // Default GRPC address
}

func (s *serverManagerAdapter) GetGRPCServer() *grpc.Server {
	return s.commonServerManager.GetGRPCServer()
}

func (s *serverManagerAdapter) GetHTTPServer() *http.Server {
	return s.commonServerManager.GetHTTPServer()
}

func (s *serverManagerAdapter) GetHTTPAddress() string {
	return s.commonServerManager.GetHTTPAddress()
}

func (s *serverManagerAdapter) Start(ctx context.Context) error {
	return s.commonServerManager.Start(ctx)
}

func (s *serverManagerAdapter) Stop(ctx context.Context) error {
	return s.commonServerManager.Stop(ctx)
}

func (s *serverManagerAdapter) IsHealthy(ctx context.Context) bool {
	// TODO: Implement proper health check logic
	return true
}

func (s *serverManagerAdapter) IsRunning() bool {
	// TODO: Implement proper running check logic
	return true
}


func (a *authLoggerAdapter) WithField(key string, value interface{}) frameauth.Logger {
	return &authLoggerAdapter{logger: a.logger.WithField(key, value)}
}

func (a *authLoggerAdapter) WithError(err error) frameauth.Logger {
	return &authLoggerAdapter{logger: a.logger.WithError(err)}
}

func (a *authLoggerAdapter) Debug(args ...interface{}) {
	if len(args) > 0 {
		if msg, ok := args[0].(string); ok {
			a.logger.Debug(msg)
		} else {
			a.logger.Debug("debug message", args...)
		}
	}
}

func (a *authLoggerAdapter) Info(args ...interface{}) {
	if len(args) > 0 {
		if msg, ok := args[0].(string); ok {
			a.logger.Info(msg)
		} else {
			a.logger.Info("info message", args...)
		}
	}
}

func (a *authLoggerAdapter) Warn(args ...interface{}) {
	if len(args) > 0 {
		if msg, ok := args[0].(string); ok {
			a.logger.Warn(msg)
		} else {
			a.logger.Warn("warning message", args...)
		}
	}
}

func (a *authLoggerAdapter) Error(args ...interface{}) {
	if len(args) > 0 {
		if msg, ok := args[0].(string); ok {
			a.logger.Error(msg)
		} else {
			a.logger.Error("error message", args...)
		}
	}
}

// Simple config adapters for observability module
type simpleTracingConfig struct {
	serviceName    string
	serviceVersion string
	environment    string
	enableTracing  bool
}

func (c *simpleTracingConfig) ServiceName() string    { return c.serviceName }
func (c *simpleTracingConfig) ServiceVersion() string { return c.serviceVersion }
func (c *simpleTracingConfig) Environment() string    { return c.environment }
func (c *simpleTracingConfig) EnableTracing() bool    { return c.enableTracing }

type simpleLoggingConfig struct{}

func (c *simpleLoggingConfig) LoggingLevel() string      { return "info" }
func (c *simpleLoggingConfig) LoggingColored() bool      { return true }
func (c *simpleLoggingConfig) LoggingFormat() string     { return "text" }
func (c *simpleLoggingConfig) LoggingTimeFormat() string { return "2006-01-02T15:04:05Z07:00" }

// WithAutoConfiguration automatically enables modules based on configuration values
func WithAutoConfiguration() Option {
	return func(ctx context.Context, service Service) {
		config := service.Config()
		if config == nil {
			return
		}

		detector := NewModuleConfigDetector(config)
		logger := service.Log(ctx)

		// Auto-enable modules based on configuration
		if detector.IsAuthenticationEnabled() {
			logger.Info("Auto-enabling authentication module based on configuration")
			WithAuthentication()(ctx, service)
		}

		if detector.IsAuthorizationEnabled() {
			logger.Info("Auto-enabling authorization module based on configuration")
			WithAuthorization()(ctx, service)
		}

		if detector.IsDataEnabled() {
			logger.Info("Auto-enabling data module based on configuration")
			WithDataModule()(ctx, service)
		}

		if detector.IsQueueEnabled() {
			logger.Info("Auto-enabling queue module based on configuration")
			WithQueue()(ctx, service)
		}

		if detector.IsObservabilityEnabled() {
			logger.Info("Auto-enabling observability module based on configuration")
			WithObservability()(ctx, service)
		}

		if detector.IsServerEnabled() {
			logger.Info("Auto-enabling server module based on configuration")
			WithServer()(ctx, service)
		}
	}
}

// WithAuthentication enables the authentication module
func WithAuthentication() Option {
	return func(ctx context.Context, service Service) {
		config := service.Config()
		logger := service.Log(ctx)

		// Create authenticator based on configuration
		// Create logger adapter for frameauth.Logger interface
		loggerAdapter := &authLoggerAdapter{logger: logger}
		authenticator := frameauth.NewAuthenticator(
			config.(frameauth.Config),
			loggerAdapter,
		)

		// Create and register the authentication module with JWT client map
		jwtClient := make(map[string]any)
		authModule := NewAuthModule(authenticator, jwtClient)
		if err := service.RegisterModule(authModule); err != nil {
			logger.WithError(err).Error("Failed to register authentication module")
			return
		}

		logger.Info("Authentication module enabled and registered")
	}
}

// WithAuthorization enables the authorization module
func WithAuthorization() Option {
	return func(ctx context.Context, service Service) {
		config := service.Config()
		logger := service.Log(ctx)

		// Create authorizer based on configuration
		authorizer := frameauthorization.NewAuthorizer(
			config.(frameauthorization.Config),
			nil, // HTTP client will be injected by service
			nil, // Temporarily use nil until logger adapter is implemented
		)

		// Create and register the authorization module
		authzModule := NewAuthzModule(authorizer)
		if err := service.RegisterModule(authzModule); err != nil {
			logger.WithError(err).Error("Failed to register authorization module")
			return
		}

		logger.Info("Authorization module enabled and registered")
	}
}

// WithDataModule enables the data module
func WithDataModule() Option {
	return func(ctx context.Context, service Service) {
		config := service.Config()
		logger := service.Log(ctx)

		// Create datastore manager based on configuration
		datastoreManager := common.NewDatastoreManager(
			config,
			logger,
			nil, // Metrics collector will be injected by service
		)

		// Create migrator
		migrator := common.NewMigrator(
			datastoreManager,
			config,
			logger,
			nil, // Filesystem will be injected by service
		)

		// Create and register the data module with dataStores map
		dataStores := &sync.Map{}
		// Type assert interface{} to concrete types for NewDataModule
		datastoreManagerConcrete, ok1 := datastoreManager.(framedata.DatastoreManager)
		migratorConcrete, ok2 := migrator.(framedata.Migrator)
		if !ok1 || !ok2 {
			logger.Error("Failed to type assert datastore manager or migrator")
			return
		}
		dataModule := NewDataModule(datastoreManagerConcrete, migratorConcrete, dataStores)
		if err := service.RegisterModule(dataModule); err != nil {
			logger.WithError(err).Error("Failed to register data module")
			return
		}

		logger.Info("Data module enabled and registered")
	}
}

// WithQueue enables the queue module
func WithQueue() Option {
	return func(ctx context.Context, service Service) {
		config := service.Config()
		logger := service.Log(ctx)

		// Create queue manager based on configuration
		queueManager := framequeue.NewQueueManager(
			config.(framequeue.Config),
			nil, // Temporarily use nil until logger adapter is implemented
		)

		// Create and register the queue module with queue and event registry
		queue := interface{}(nil) // Initialize empty queue
		eventRegistry := make(map[string]interface{})
		queueModule := NewQueueModule(queueManager, queue, eventRegistry)
		if err := service.RegisterModule(queueModule); err != nil {
			logger.WithError(err).Error("Failed to register queue module")
			return
		}

		logger.Info("Queue module enabled and registered")
	}
}

// WithObservability enables the observability module
func WithObservability() Option {
	return func(ctx context.Context, service Service) {
		logger := service.Log(ctx)

		// Create observability manager based on configuration
		config := service.Config()
		
		// Extract service configuration with sensible defaults
		serviceName := "frame-service"
		serviceVersion := "1.0.0"
		environment := "development"
		
		// Try to get values from config if available
		if configurable, ok := config.(interface {
			GetServiceName() string
			GetServiceVersion() string
			GetEnvironment() string
		}); ok {
			if name := configurable.GetServiceName(); name != "" {
				serviceName = name
			}
			if version := configurable.GetServiceVersion(); version != "" {
				serviceVersion = version
			}
			if env := configurable.GetEnvironment(); env != "" {
				environment = env
			}
		}
		
		tracingConfig := &simpleTracingConfig{
			serviceName:    serviceName,
			serviceVersion: serviceVersion,
			environment:    environment,
			enableTracing:  true,
		}
		loggingConfig := &simpleLoggingConfig{}
		
		obsManager := frameobservability.NewManager(
			tracingConfig,
			loggingConfig,
			frameobservability.ObservabilityOptions{
				EnableTracing: true, // Will be configured based on config
				Logger:        logger,
			},
		)

		// Create and register the observability module with tracing parameters
		enableTracing := true // Will be configured based on config
		obsModule := NewObservabilityModule(obsManager, enableTracing, nil, nil, nil, nil, nil)
		if err := service.RegisterModule(obsModule); err != nil {
			logger.WithError(err).Error("Failed to register observability module")
			return
		}

		logger.Info("Observability module enabled and registered")
	}
}

// WithServer enables the server module
func WithServer() Option {
	return func(ctx context.Context, service Service) {
		config := service.Config()
		logger := service.Log(ctx)

		// Create server manager based on configuration
		serverManager := common.NewServerManager(
			config,
			logger,
			nil, // Temporarily use nil until metrics collector adapter is implemented
		)

		// Create and register the server module with server parameters
		// Type assert common.ServerManager to frameserver.ServerManager using adapter
		serverManagerAdapter := &serverManagerAdapter{commonServerManager: serverManager}
		serverModule := NewServerModule(serverManagerAdapter, nil, nil, false, nil, nil, "", nil)
		if err := service.RegisterModule(serverModule); err != nil {
			logger.WithError(err).Error("Failed to register server module")
			return
		}

		logger.Info("Server module enabled and registered")
	}
}

// RequireModule ensures a specific module is enabled, returning an error if not configured
func RequireModule(moduleName string) Option {
	return func(ctx context.Context, service Service) {
		config := service.Config()
		detector := NewModuleConfigDetector(config)
		logger := service.Log(ctx)

		var isEnabled bool
		switch moduleName {
		case "authentication":
			isEnabled = detector.IsAuthenticationEnabled()
		case "authorization":
			isEnabled = detector.IsAuthorizationEnabled()
		case "data":
			isEnabled = detector.IsDataEnabled()
		case "queue":
			isEnabled = detector.IsQueueEnabled()
		case "observability":
			isEnabled = detector.IsObservabilityEnabled()
		case "server":
			isEnabled = detector.IsServerEnabled()
		default:
			logger.WithField("module", moduleName).Error("Unknown module name")
			return
		}

		if !isEnabled {
			logger.WithField("module", moduleName).Error("Required module is not configured")
			panic("Required module " + moduleName + " is not configured")
		}

		logger.WithField("module", moduleName).Info("Required module is properly configured")
	}
}
