package frame

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pitabwire/util"
	"github.com/pitabwire/frame/internal/frameauth"
	"github.com/pitabwire/frame/internal/frameauthorization"
	"github.com/pitabwire/frame/internal/framedata"
	"github.com/pitabwire/frame/internal/framequeue"
	"gorm.io/gorm"
	"github.com/pitabwire/frame/internal/frameobservability"
	"github.com/pitabwire/frame/internal/frameserver"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
)

// AuthModule wraps the frameauth.Authenticator to implement the Module interface
type AuthModule struct {
	authenticator frameauth.Authenticator
	jwtClient     map[string]any
	status        ModuleStatus
	enabled       bool
}

// NewAuthModule creates a new authentication module with JWT client parameters
func NewAuthModule(authenticator frameauth.Authenticator, jwtClient map[string]any) *AuthModule {
	return &AuthModule{
		authenticator: authenticator,
		jwtClient:     jwtClient,
		status:        ModuleStatusLoaded,
		enabled:       authenticator != nil && authenticator.IsEnabled(),
	}
}

func (m *AuthModule) Type() ModuleType                { return ModuleTypeAuthentication }
func (m *AuthModule) Name() string                    { return "Authentication Module" }
func (m *AuthModule) Version() string                 { return "1.0.0" }
func (m *AuthModule) Status() ModuleStatus            { return m.status }
func (m *AuthModule) Dependencies() []ModuleType      { return []ModuleType{} }
func (m *AuthModule) IsEnabled() bool                 { return m.enabled }
func (m *AuthModule) Authenticator() frameauth.Authenticator { return m.authenticator }

func (m *AuthModule) Initialize(ctx context.Context, config any) error {
	m.status = ModuleStatusLoaded
	return nil
}

func (m *AuthModule) Start(ctx context.Context) error {
	m.status = ModuleStatusStarted
	return nil
}

func (m *AuthModule) Stop(ctx context.Context) error {
	m.status = ModuleStatusStopped
	return nil
}

func (m *AuthModule) HealthCheck() error {
	if m.authenticator == nil {
		return fmt.Errorf("authenticator not initialized")
	}
	return nil
}

// AuthzModule wraps the frameauthorization.Authorizer to implement the Module interface
type AuthzModule struct {
	authorizer frameauthorization.Authorizer
	status     ModuleStatus
	enabled    bool
}

func NewAuthzModule(authorizer frameauthorization.Authorizer) *AuthzModule {
	return &AuthzModule{
		authorizer: authorizer,
		status:     ModuleStatusLoaded,
		enabled:    authorizer != nil && authorizer.IsEnabled(),
	}
}

func (m *AuthzModule) Type() ModuleType                { return ModuleTypeAuthorization }
func (m *AuthzModule) Name() string                    { return "Authorization Module" }
func (m *AuthzModule) Version() string                 { return "1.0.0" }
func (m *AuthzModule) Status() ModuleStatus            { return m.status }
func (m *AuthzModule) Dependencies() []ModuleType      { return []ModuleType{} }
func (m *AuthzModule) IsEnabled() bool                 { return m.enabled }
func (m *AuthzModule) Authorizer() frameauthorization.Authorizer { return m.authorizer }

func (m *AuthzModule) Initialize(ctx context.Context, config any) error {
	m.status = ModuleStatusLoaded
	return nil
}

func (m *AuthzModule) Start(ctx context.Context) error {
	m.status = ModuleStatusStarted
	return nil
}

func (m *AuthzModule) Stop(ctx context.Context) error {
	m.status = ModuleStatusStopped
	return nil
}

func (m *AuthzModule) HealthCheck() error {
	if m.authorizer == nil {
		return fmt.Errorf("authorizer not initialized")
	}
	return nil
}

// DataModule wraps the framedata components to implement the Module interface
type DataModule struct {
	datastoreManager framedata.DatastoreManager
	migrator         framedata.Migrator
	dataStores       *sync.Map
	status           ModuleStatus
	enabled          bool
}

func NewDataModule(datastoreManager framedata.DatastoreManager, migrator framedata.Migrator, dataStores *sync.Map) *DataModule {
	if dataStores == nil {
		dataStores = &sync.Map{}
	}
	return &DataModule{
		datastoreManager: datastoreManager,
		migrator:         migrator,
		dataStores:       dataStores,
		status:           ModuleStatusLoaded,
		enabled:          datastoreManager != nil,
	}
}

func (m *DataModule) Type() ModuleType                { return ModuleTypeData }
func (m *DataModule) Name() string                    { return "Data Module" }
func (m *DataModule) Version() string                 { return "1.0.0" }
func (m *DataModule) Status() ModuleStatus            { return m.status }
func (m *DataModule) Dependencies() []ModuleType      { return []ModuleType{} }
func (m *DataModule) IsEnabled() bool                 { return m.enabled }
func (m *DataModule) DatastoreManager() framedata.DatastoreManager { return m.datastoreManager }
func (m *DataModule) Migrator() framedata.Migrator    { return m.migrator }
func (m *DataModule) DataStores() *sync.Map           { return m.dataStores }

func (m *DataModule) SearchProvider() framedata.SearchProvider {
	// SearchProvider functionality is handled separately from DatastoreManager
	// This method returns nil as the search provider is managed independently
	return nil
}

func (m *DataModule) Initialize(ctx context.Context, config any) error {
	m.status = ModuleStatusLoaded
	return nil
}

func (m *DataModule) Start(ctx context.Context) error {
	m.status = ModuleStatusStarted
	return nil
}

func (m *DataModule) Stop(ctx context.Context) error {
	m.status = ModuleStatusStopped
	return nil
}

// DatastoreService interface methods for DataModule
func (m *DataModule) DBPool(name ...string) interface{} {
	// TODO: Implement proper DBPool access via datastoreManager
	return nil
}

func (m *DataModule) DB(ctx context.Context, readOnly bool) *gorm.DB {
	// TODO: Implement proper DB access via datastoreManager
	return nil
}

func (m *DataModule) DBWithName(ctx context.Context, name string, readOnly bool) *gorm.DB {
	// TODO: Implement proper DBWithName access via datastoreManager
	return nil
}

func (m *DataModule) HealthCheck() error {
	if m.datastoreManager == nil {
		return fmt.Errorf("datastore manager not initialized")
	}
	return nil
}

// QueueModule wraps the framequeue.QueueManager to implement the Module interface
type QueueModule struct {
	queueManager  framequeue.QueueManager
	queue         interface{}
	eventRegistry map[string]interface{}
	status        ModuleStatus
	enabled       bool
}

func NewQueueModule(queueManager framequeue.QueueManager, q interface{}, eventRegistry map[string]interface{}) *QueueModule {
	if eventRegistry == nil {
		eventRegistry = make(map[string]interface{})
	}
	return &QueueModule{
		queueManager:  queueManager,
		queue:         q,
		eventRegistry: eventRegistry,
		status:        ModuleStatusLoaded,
		enabled:       queueManager != nil,
	}
}

func (m *QueueModule) Type() ModuleType                { return ModuleTypeQueue }
func (m *QueueModule) Name() string                    { return "Queue Module" }
func (m *QueueModule) Version() string                 { return "1.0.0" }
func (m *QueueModule) Status() ModuleStatus            { return m.status }
func (m *QueueModule) Dependencies() []ModuleType      { return []ModuleType{} }
func (m *QueueModule) IsEnabled() bool                 { return m.enabled }
func (m *QueueModule) QueueManager() framequeue.QueueManager { return m.queueManager }
func (m *QueueModule) Queue() interface{}                        { return m.queue }
func (m *QueueModule) EventRegistry() map[string]interface{}    { return m.eventRegistry }

func (m *QueueModule) Initialize(ctx context.Context, config any) error {
	m.status = ModuleStatusLoaded
	return nil
}

func (m *QueueModule) Start(ctx context.Context) error {
	m.status = ModuleStatusStarted
	return nil
}

func (m *QueueModule) Stop(ctx context.Context) error {
	m.status = ModuleStatusStopped
	return nil
}

func (m *QueueModule) HealthCheck() error {
	if m.queueManager == nil {
		return fmt.Errorf("queue manager not initialized")
	}
	return nil
}

// ObservabilityModule wraps the frameobservability.ObservabilityManager to implement the Module interface
type ObservabilityModule struct {
	observabilityManager frameobservability.ObservabilityManager
	enableTracing        bool
	traceTextMap         propagation.TextMapPropagator
	traceExporter        trace.SpanExporter
	traceSampler         trace.Sampler
	metricsReader        metric.Reader
	traceLogsExporter    log.Exporter
	status               ModuleStatus
	enabled              bool
}

func NewObservabilityModule(observabilityManager frameobservability.ObservabilityManager, enableTracing bool,
	traceTextMap propagation.TextMapPropagator, traceExporter trace.SpanExporter, traceSampler trace.Sampler,
	metricsReader metric.Reader, traceLogsExporter log.Exporter) *ObservabilityModule {
	return &ObservabilityModule{
		observabilityManager: observabilityManager,
		enableTracing:        enableTracing,
		traceTextMap:         traceTextMap,
		traceExporter:        traceExporter,
		traceSampler:         traceSampler,
		metricsReader:        metricsReader,
		traceLogsExporter:    traceLogsExporter,
		status:               ModuleStatusLoaded,
		enabled:              observabilityManager != nil,
	}
}

func (m *ObservabilityModule) Type() ModuleType                { return ModuleTypeObservability }
func (m *ObservabilityModule) Name() string                    { return "Observability Module" }
func (m *ObservabilityModule) Version() string                 { return "1.0.0" }
func (m *ObservabilityModule) Status() ModuleStatus            { return m.status }
func (m *ObservabilityModule) Dependencies() []ModuleType      { return []ModuleType{} }
func (m *ObservabilityModule) IsEnabled() bool                 { return m.enabled }
func (m *ObservabilityModule) ObservabilityManager() frameobservability.ObservabilityManager { return m.observabilityManager }
func (m *ObservabilityModule) EnableTracing() bool                                        { return m.enableTracing }
func (m *ObservabilityModule) TraceTextMap() propagation.TextMapPropagator               { return m.traceTextMap }
func (m *ObservabilityModule) TraceExporter() trace.SpanExporter                         { return m.traceExporter }
func (m *ObservabilityModule) TraceSampler() trace.Sampler                               { return m.traceSampler }
func (m *ObservabilityModule) MetricsReader() metric.Reader                              { return m.metricsReader }
func (m *ObservabilityModule) TraceLogsExporter() log.Exporter                           { return m.traceLogsExporter }

func (m *ObservabilityModule) Initialize(ctx context.Context, config any) error {
	m.status = ModuleStatusLoaded
	return nil
}

func (m *ObservabilityModule) Start(ctx context.Context) error {
	m.status = ModuleStatusStarted
	return nil
}

func (m *ObservabilityModule) Stop(ctx context.Context) error {
	m.status = ModuleStatusStopped
	return nil
}

func (m *ObservabilityModule) HealthCheck() error {
	if m.observabilityManager == nil {
		return fmt.Errorf("observability manager not initialized")
	}
	return nil
}

// ServerModule wraps the frameserver.ServerManager to implement the Module interface
type ServerModule struct {
	serverManager              frameserver.ServerManager
	handler                    http.Handler
	grpcServer                 *grpc.Server
	grpcServerEnableReflection bool
	priListener                net.Listener
	secListener                net.Listener
	grpcPort                   string
	driver                     any
	status                     ModuleStatus
	enabled                    bool
}

func NewServerModule(serverManager frameserver.ServerManager, handler http.Handler, grpcServer *grpc.Server, 
	grpcServerEnableReflection bool, priListener, secListener net.Listener, grpcPort string, driver any) *ServerModule {
	return &ServerModule{
		serverManager:              serverManager,
		handler:                    handler,
		grpcServer:                 grpcServer,
		grpcServerEnableReflection: grpcServerEnableReflection,
		priListener:                priListener,
		secListener:                secListener,
		grpcPort:                   grpcPort,
		driver:                     driver,
		status:                     ModuleStatusLoaded,
		enabled:                    serverManager != nil,
	}
}

func (m *ServerModule) Type() ModuleType                { return ModuleTypeServer }
func (m *ServerModule) Name() string                    { return "Server Module" }
func (m *ServerModule) Version() string                 { return "1.0.0" }
func (m *ServerModule) Status() ModuleStatus            { return m.status }
func (m *ServerModule) Dependencies() []ModuleType      { return []ModuleType{} }
func (m *ServerModule) IsEnabled() bool                 { return m.enabled }
func (m *ServerModule) ServerManager() frameserver.ServerManager { return m.serverManager }
func (m *ServerModule) Handler() http.Handler                   { return m.handler }
func (m *ServerModule) GRPCServer() *grpc.Server                { return m.grpcServer }
func (m *ServerModule) GRPCServerEnableReflection() bool        { return m.grpcServerEnableReflection }
func (m *ServerModule) PrimaryListener() net.Listener           { return m.priListener }
func (m *ServerModule) SecondaryListener() net.Listener         { return m.secListener }
func (m *ServerModule) GRPCPort() string                        { return m.grpcPort }
func (m *ServerModule) Driver() any                             { return m.driver }

func (m *ServerModule) Initialize(ctx context.Context, config any) error {
	m.status = ModuleStatusLoaded
	return nil
}

func (m *ServerModule) Start(ctx context.Context) error {
	m.status = ModuleStatusStarted
	return nil
}

func (m *ServerModule) Stop(ctx context.Context) error {
	m.status = ModuleStatusStopped
	return nil
}

func (m *ServerModule) HealthCheck() error {
	if m.serverManager == nil {
		return fmt.Errorf("server manager not initialized")
	}
	return nil
}

// WorkerPoolModule manages worker pool parameters
type WorkerPoolModule struct {
	pool             interface{}
	poolOptions      interface{}
	backGroundClient func(ctx context.Context) error
	status           ModuleStatus
	enabled          bool
}

func NewWorkerPoolModule(pool interface{}, poolOptions interface{}, backGroundClient func(ctx context.Context) error) *WorkerPoolModule {
	return &WorkerPoolModule{
		pool:             pool,
		poolOptions:      poolOptions,
		backGroundClient: backGroundClient,
		status:           ModuleStatusLoaded,
		enabled:          pool != nil,
	}
}

func (m *WorkerPoolModule) Type() ModuleType                { return ModuleTypeWorkerPool }
func (m *WorkerPoolModule) Name() string                    { return "Worker Pool Module" }
func (m *WorkerPoolModule) Version() string                 { return "1.0.0" }
func (m *WorkerPoolModule) Status() ModuleStatus            { return m.status }
func (m *WorkerPoolModule) Dependencies() []ModuleType      { return []ModuleType{} }
func (m *WorkerPoolModule) IsEnabled() bool                 { return m.enabled }
func (m *WorkerPoolModule) Pool() interface{}                { return m.pool }
func (m *WorkerPoolModule) PoolOptions() interface{} { return m.poolOptions }
func (m *WorkerPoolModule) BackGroundClient() func(ctx context.Context) error { return m.backGroundClient }

func (m *WorkerPoolModule) Initialize(ctx context.Context, config any) error {
	m.status = ModuleStatusLoaded
	return nil
}

func (m *WorkerPoolModule) Start(ctx context.Context) error {
	m.status = ModuleStatusStarted
	return nil
}

func (m *WorkerPoolModule) Stop(ctx context.Context) error {
	m.status = ModuleStatusStopped
	return nil
}

func (m *WorkerPoolModule) HealthCheck() error {
	if m.pool == nil {
		return fmt.Errorf("worker pool not initialized")
	}
	return nil
}

// HealthModule manages health check parameters
type HealthModule struct {
	healthCheckers  []interface{}
	healthCheckPath string
	status          ModuleStatus
	enabled         bool
}

func NewHealthModule(healthCheckers []interface{}, healthCheckPath string) *HealthModule {
	return &HealthModule{
		healthCheckers:  healthCheckers,
		healthCheckPath: healthCheckPath,
		status:          ModuleStatusLoaded,
		enabled:         len(healthCheckers) > 0,
	}
}

func (m *HealthModule) Type() ModuleType                { return ModuleTypeHealth }
func (m *HealthModule) Name() string                    { return "Health Module" }
func (m *HealthModule) Version() string                 { return "1.0.0" }
func (m *HealthModule) Status() ModuleStatus            { return m.status }
func (m *HealthModule) Dependencies() []ModuleType      { return []ModuleType{} }
func (m *HealthModule) IsEnabled() bool                 { return m.enabled }
func (m *HealthModule) HealthCheckers() []interface{}       { return m.healthCheckers }
func (m *HealthModule) HealthCheckPath() string         { return m.healthCheckPath }

func (m *HealthModule) Initialize(ctx context.Context, config any) error {
	m.status = ModuleStatusLoaded
	return nil
}

func (m *HealthModule) Start(ctx context.Context) error {
	m.status = ModuleStatusStarted
	return nil
}

func (m *HealthModule) Stop(ctx context.Context) error {
	m.status = ModuleStatusStopped
	return nil
}

func (m *HealthModule) HealthCheck() error {
	return nil // Health module itself is always healthy
}

// LoggingModule manages logging parameters
type LoggingModule struct {
	logger  *util.LogEntry
	status  ModuleStatus
	enabled bool
}

func NewLoggingModule(logger *util.LogEntry) *LoggingModule {
	return &LoggingModule{
		logger:  logger,
		status:  ModuleStatusLoaded,
		enabled: logger != nil,
	}
}

func (m *LoggingModule) Type() ModuleType                { return ModuleTypeLogging }
func (m *LoggingModule) Name() string                    { return "Logging Module" }
func (m *LoggingModule) Version() string                 { return "1.0.0" }
func (m *LoggingModule) Status() ModuleStatus            { return m.status }
func (m *LoggingModule) Dependencies() []ModuleType      { return []ModuleType{} }
func (m *LoggingModule) IsEnabled() bool                 { return m.enabled }
func (m *LoggingModule) Logger() *util.LogEntry         { return m.logger }

func (m *LoggingModule) Initialize(ctx context.Context, config any) error {
	m.status = ModuleStatusLoaded
	return nil
}

func (m *LoggingModule) Start(ctx context.Context) error {
	m.status = ModuleStatusStarted
	return nil
}

func (m *LoggingModule) Stop(ctx context.Context) error {
	m.status = ModuleStatusStopped
	return nil
}

func (m *LoggingModule) Cleanup(ctx context.Context) error {
	m.status = ModuleStatusUnloaded
	return nil
}

func (m *LoggingModule) HealthCheck() error {
	if m.logger == nil {
		return fmt.Errorf("logger not initialized")
	}
	return nil
}

// LocalizationModule manages i18n bundle parameters
type LocalizationModule struct {
	bundle  *i18n.Bundle
	status  ModuleStatus
	enabled bool
}

func NewLocalizationModule(bundle *i18n.Bundle) *LocalizationModule {
	return &LocalizationModule{
		bundle:  bundle,
		status:  ModuleStatusLoaded,
		enabled: bundle != nil,
	}
}

func (m *LocalizationModule) Type() ModuleType                { return ModuleTypeLocalization }
func (m *LocalizationModule) Name() string                    { return "Localization Module" }
func (m *LocalizationModule) Version() string                 { return "1.0.0" }
func (m *LocalizationModule) Status() ModuleStatus            { return m.status }
func (m *LocalizationModule) Dependencies() []ModuleType      { return []ModuleType{} }
func (m *LocalizationModule) IsEnabled() bool                 { return m.enabled }
func (m *LocalizationModule) Bundle() *i18n.Bundle           { return m.bundle }

func (m *LocalizationModule) Initialize(ctx context.Context, config any) error {
	m.status = ModuleStatusLoaded
	return nil
}

func (m *LocalizationModule) Start(ctx context.Context) error {
	m.status = ModuleStatusStarted
	return nil
}

func (m *LocalizationModule) Stop(ctx context.Context) error {
	m.status = ModuleStatusStopped
	return nil
}

func (m *LocalizationModule) HealthCheck() error {
	if m.bundle == nil {
		return fmt.Errorf("i18n bundle not initialized")
	}
	return nil
}
