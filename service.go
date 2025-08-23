package frame

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/signal"
	"sync"
	"syscall"
	"time"

	ghandler "github.com/gorilla/handlers"
	"github.com/pitabwire/util"
	"gorm.io/gorm"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	_ "go.uber.org/automaxprocs" // Automatically set GOMAXPROCS to match Linux container CPU quota.
	"google.golang.org/grpc/reflection"

	"github.com/pitabwire/frame/internal"
	"github.com/pitabwire/frame/internal/common"
)

type contextKey string

func (c contextKey) String() string {
	return "frame/" + string(c)
}

const (
	ctxKeyService = contextKey("serviceKey")

	defaultHTTPReadTimeoutSeconds  = 15
	defaultHTTPWriteTimeoutSeconds = 15
	defaultHTTPIdleTimeoutSeconds  = 60
)

// serviceImpl is the internal implementation of the Service interface
// An instance of this type scoped to stay for the lifetime of the application.
// It is pushed and pulled from contexts to make it easy to pass around.
type serviceImpl struct {
	// Core service parameters
	name          string
	version       string
	environment   string
	logger        *util.LogEntry
	configuration any
	
	// Shared HTTP client
	client *http.Client
	
	// Lifecycle management
	cancelFunc        context.CancelFunc
	errorChannelMutex sync.Mutex
	errorChannel      chan error
	startOnce         sync.Once
	stopMutex         sync.Mutex
	
	// Service hooks
	startup func(ctx context.Context, s Service)
	cleanup func(ctx context.Context)
	
	// Module registry for plugin management
	moduleRegistry *ModuleRegistry
}

// Note: Option type is now defined in interface.go

// NewService creates a new instance of Service with the name and supplied options.
// Internally it calls NewServiceWithContext and creates a background context for use.
func NewService(name string, opts ...Option) (context.Context, Service) {
	ctx := context.Background()
	return NewServiceWithContext(ctx, name, opts...)
}

// NewServiceWithContext creates a new instance of Service with context, name and supplied options
// It is used together with the Init option to setup components of a service that is not yet running.
func NewServiceWithContext(ctx context.Context, name string, opts ...Option) (context.Context, Service) {
	// Create a new context that listens for OS signals for graceful shutdown.
	ctx, signalCancelFunc := signal.NotifyContext(ctx,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	defaultLogger := util.Log(ctx)
	ctx = util.ContextWithLogger(ctx, defaultLogger)

	defaultCfg, _ := ConfigFromEnv[ConfigurationDefault]()

	// Worker pool and queue are now managed by modules

	service := &serviceImpl{
		name:         name,
		cancelFunc:   signalCancelFunc, // Store its cancel function
		errorChannel: make(chan error, 1),
		client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		logger: defaultLogger,
		
		// Initialize module registry
		moduleRegistry: NewModuleRegistry(),
	}

	if defaultCfg.ServiceName != "" {
		opts = append(opts, WithName(defaultCfg.ServiceName))
	}

	if defaultCfg.ServiceEnvironment != "" {
		opts = append(opts, WithEnvironment(defaultCfg.ServiceEnvironment))
	}

	if defaultCfg.ServiceVersion != "" {
		opts = append(opts, WithVersion(defaultCfg.ServiceVersion))
	}

	// opts = append(opts, WithLogger()) // TODO: Implement WithLogger function

	service.Init(ctx, opts...) // Apply all options, using the signal-aware context

	// Prepare context to be returned, embedding service and config
	ctx = SvcToContext(ctx, service)
	ctx = ConfigToContext(ctx, service.Config())
	ctx = util.ContextWithLogger(ctx, service.logger)
	return ctx, service
}

// SvcToContext pushes a service instance into the supplied context for easier propagation.
func SvcToContext(ctx context.Context, service Service) context.Context {
	return context.WithValue(ctx, ctxKeyService, service)
}

// Svc obtains a service instance being propagated through the context.
func Svc(ctx context.Context) Service {
	service, ok := ctx.Value(ctxKeyService).(Service)
	if !ok {
		return nil
	}

	return service
}

// Name gets the name of the service. Its the first argument used when NewService is called.
func (s *serviceImpl) Name() string {
	return s.name
}

// WithName specifies the name the service will utilize.
func WithName(name string) Option {
	return func(_ context.Context, s Service) {
		if impl, ok := s.(*serviceImpl); ok {
			impl.name = name
		}
	}
}

// Version gets the release version of the service.
func (s *serviceImpl) Version() string {
	return s.version
}

// WithVersion specifies the version the service will utilize.
func WithVersion(version string) Option {
	return func(_ context.Context, s Service) {
		if impl, ok := s.(*serviceImpl); ok {
			impl.version = version
		}
	}
}

// Environment gets the runtime environment of the service.
func (s *serviceImpl) Environment() string {
	return s.environment
}

// WithEnvironment specifies the environment the service will utilize.
func WithEnvironment(environment string) Option {
	return func(_ context.Context, s Service) {
		if impl, ok := s.(*serviceImpl); ok {
			impl.environment = environment
		}
	}
}

// JwtClient gets the authenticated jwt client if configured at startup.
func (s *serviceImpl) JwtClient() map[string]any {
	if authModule, ok := s.GetModule(ModuleTypeAuthentication).(*AuthModule); ok {
		return authModule.jwtClient
	}
	return make(map[string]any)
}

// SetJwtClient sets the authenticated jwt client.
func (s *serviceImpl) SetJwtClient(jwtCli map[string]any) {
	if authModule, ok := s.GetModule(ModuleTypeAuthentication).(*AuthModule); ok {
		// Update the existing AuthModule with the new JWT client
		*authModule = *NewAuthModule(
			authModule.Authenticator(),
			jwtCli, // Updated JWT client
		)
	}
}

// JwtClientID gets the authenticated JWT client ID if configured at startup.
func (s *serviceImpl) JwtClientID() string {
	jwtClient := s.JwtClient()
	clientID, ok := jwtClient["client_id"].(string)
	if ok {
		return clientID
	}
	oauth2Config, sok := s.Config().(ConfigurationOAUTH2)
	if sok {
		clientID = oauth2Config.GetOauth2ServiceClientID()
		if clientID != "" {
			return clientID
		}
	}

	clientID = s.Name()
	if s.Environment() != "" {
		clientID = fmt.Sprintf("%s_%s", s.Name(), s.Environment())
	}

	return clientID
}

// Config gets the configuration object associated with the service.
func (s *serviceImpl) Config() any {
	return s.configuration
}

// Log method is now implemented in logger.go using LoggingModule

// JwtClientSecret gets the authenticated jwt client if configured at startup.
func (s *serviceImpl) JwtClientSecret() string {
	jwtClient := s.JwtClient()
	clientSecret, ok := jwtClient["client_secret"].(string)
	if ok {
		return clientSecret
	}
	oauth2Config, sok := s.Config().(ConfigurationOAUTH2)
	if sok {
		return oauth2Config.GetOauth2ServiceClientSecret()
	}
	return ""
}

func (s *serviceImpl) H() http.Handler {
	if serverModule, ok := s.GetModule(ModuleTypeServer).(*ServerModule); ok {
		return serverModule.Handler()
	}
	return nil
}

// Init evaluates the options provided as arguments and supplies them to the service object.
func (s *serviceImpl) Init(ctx context.Context, opts ...Option) {
	for _, opt := range opts {
		opt(ctx, s)
	}
}

// AddPreStartMethod Adds user defined functions that can be run just before
// the service starts receiving requests but is fully initialized.
func (s *serviceImpl) AddPreStartMethod(f func(ctx context.Context, s Service)) {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()
	if s.startup == nil {
		s.startup = f
		return
	}

	old := s.startup
	s.startup = func(ctx context.Context, st Service) { old(ctx, st); f(ctx, st) }
}

// AddCleanupMethod Adds user defined functions to be run just before completely stopping the service.
// These are responsible for properly and gracefully stopping active components.
func (s *serviceImpl) AddCleanupMethod(f func(ctx context.Context)) {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	if s.cleanup == nil {
		s.cleanup = f
		return
	}

	old := s.cleanup
	s.cleanup = func(ctx context.Context) { f(ctx); old(ctx) }
}

// AddHealthCheck Adds health checks that are run periodically to ascertain the system is ok
// The arguments are implementations of the checker interface and should work with just about
// any system that is given to them.
func (s *serviceImpl) AddHealthCheck(checker interface{}) {
	if healthModule, ok := s.GetModule(ModuleTypeHealth).(*HealthModule); ok {
		// Update the existing HealthModule with the new health checker
		existingCheckers := healthModule.HealthCheckers()
		updatedCheckers := append(existingCheckers, checker)
		*healthModule = *NewHealthModule(updatedCheckers, healthModule.HealthCheckPath())
	}
}

// DatastoreService interface methods
func (s *serviceImpl) DBPool(name ...string) interface{} {
	if dataModule, ok := s.GetModule(ModuleTypeData).(*DataModule); ok {
		return dataModule.DBPool(name...)
	}
	return nil
}

func (s *serviceImpl) DB(ctx context.Context, readOnly bool) *gorm.DB {
	if dataModule, ok := s.GetModule(ModuleTypeData).(*DataModule); ok {
		return dataModule.DB(ctx, readOnly)
	}
	return nil
}

func (s *serviceImpl) DBWithName(ctx context.Context, name string, readOnly bool) *gorm.DB {
	if dataModule, ok := s.GetModule(ModuleTypeData).(*DataModule); ok {
		return dataModule.DBWithName(ctx, name, readOnly)
	}
	return nil
}

// HandleHealth implements the Service interface
func (s *serviceImpl) HandleHealth(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement proper health check logic
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// InvokeRestService implements the Service interface
func (s *serviceImpl) InvokeRestService(ctx context.Context, method string, url string, payload map[string]any, headers map[string][]string) (int, []byte, error) {
	// TODO: Implement proper REST service invocation
	return 200, []byte("OK"), nil
}

// InvokeRestServiceURLEncoded implements the Service interface
func (s *serviceImpl) InvokeRestServiceURLEncoded(ctx context.Context, method string, url string, payload url.Values, headers map[string]string) (int, []byte, error) {
	// TODO: Implement proper URL-encoded REST service invocation
	return 200, []byte("OK"), nil
}

// Log implements the Service interface
func (s *serviceImpl) Log(ctx context.Context) *util.LogEntry {
	return util.Log(ctx)
}


// loggerAdapter adapts util.LogEntry to common.Logger interface
type loggerAdapter struct {
	logger *util.LogEntry
}

func (l *loggerAdapter) WithField(key string, value interface{}) common.Logger {
	return &loggerAdapter{logger: l.logger.WithField(key, value)}
}

func (l *loggerAdapter) WithError(err error) common.Logger {
	return &loggerAdapter{logger: l.logger.WithError(err)}
}

func (l *loggerAdapter) Fatal(args ...interface{}) {
	if len(args) > 0 {
		if msg, ok := args[0].(string); ok && len(args) > 1 {
			l.logger.Fatal(msg, args[1:]...)
		} else {
			l.logger.Fatal("Fatal error", args...)
		}
	} else {
		l.logger.Fatal("Fatal error")
	}
}

func (l *loggerAdapter) Error(args ...interface{}) {
	if len(args) > 0 {
		if msg, ok := args[0].(string); ok && len(args) > 1 {
			l.logger.Error(msg, args[1:]...)
		} else {
			l.logger.Error("Error", args...)
		}
	} else {
		l.logger.Error("Error")
	}
}

func (l *loggerAdapter) Warn(args ...interface{}) {
	if len(args) > 0 {
		if msg, ok := args[0].(string); ok && len(args) > 1 {
			l.logger.Warn(msg, args[1:]...)
		} else {
			l.logger.Warn("Warning", args...)
		}
	} else {
		l.logger.Warn("Warning")
	}
}

func (l *loggerAdapter) Info(args ...interface{}) {
	if len(args) > 0 {
		if msg, ok := args[0].(string); ok && len(args) > 1 {
			l.logger.Info(msg, args[1:]...)
		} else {
			l.logger.Info("Info", args...)
		}
	} else {
		l.logger.Info("Info")
	}
}

func (l *loggerAdapter) Debug(args ...interface{}) {
	if len(args) > 0 {
		if msg, ok := args[0].(string); ok && len(args) > 1 {
			l.logger.Debug(msg, args[1:]...)
		} else {
			l.logger.Debug("Debug", args...)
		}
	} else {
		l.logger.Debug("Debug")
	}
}

// ModuleService interface methods
func (s *serviceImpl) Modules() *ModuleRegistry {
	return s.moduleRegistry
}

func (s *serviceImpl) GetModule(moduleType ModuleType) Module {
	return s.moduleRegistry.Get(moduleType)
}

func (s *serviceImpl) GetTypedModule(moduleType ModuleType, target interface{}) bool {
	return s.moduleRegistry.GetTyped(moduleType, target)
}

func (s *serviceImpl) RegisterModule(module Module) error {
	return s.moduleRegistry.Register(module)
}

func (s *serviceImpl) HasModule(moduleType ModuleType) bool {
	module := s.moduleRegistry.Get(moduleType)
	return module != nil && module.IsEnabled()
}

// HandleHealth method is implemented in server_health.go

// Run keeps the service useful by handling incoming requests.
func (s *serviceImpl) Run(ctx context.Context, address string) error {
	// pubSubErr := s.initPubsub(ctx) // TODO: Implement initPubsub method
	pubSubErr := error(nil) // Placeholder
	if pubSubErr != nil {
		return pubSubErr
	}

	// Background processing is now handled by modules

	go func(ctx context.Context) {
		srvErr := s.initServer(ctx, address)
		s.sendStopError(ctx, srvErr)
	}(ctx)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err0 := <-s.errorChannel:
		if err0 != nil {
			s.Log(ctx).
				WithError(err0).
				Error("system exit in error")
			s.Stop(ctx)
		} else {
			s.Log(ctx).Debug("system exit")
		}
		return err0
	}
}

func (s *serviceImpl) determineHTTPPort(currentPort string) string {
	if currentPort != "" {
		return currentPort
	}

	config, ok := s.Config().(ConfigurationPorts)
	if !ok {
		// Assuming s.TLSEnabled() checks if TLS cert/key paths are configured.
		// This part might need adjustment if s.TLSEnabled() is not available
		// or if direct check on ConfigurationTLS is preferred.
		tlsConfig, tlsOk := s.Config().(ConfigurationTLS)
		if tlsOk && tlsConfig.TLSCertPath() != "" && tlsConfig.TLSCertKeyPath() != "" {
			return ":https"
		}
		return "http" // Existing logic; consider ":http" or a default port like ":8080"
	}
	return config.HTTPPort()
}

func (s *serviceImpl) determineGRPCPort(currentPort string) string {
	if currentPort != "" {
		return currentPort
	}

	config, ok := s.Config().(ConfigurationPorts)
	if !ok {
		return ":50051" // Default gRPC port
	}
	return config.GrpcPort()
}

func (s *serviceImpl) createAndConfigureMux(_ context.Context) *http.ServeMux {
	mux := http.NewServeMux()

	// Get handler from ServerModule
	var applicationHandler http.Handler
	if serverModule, ok := s.GetModule(ModuleTypeServer).(*ServerModule); ok {
		applicationHandler = serverModule.Handler()
	}
	if applicationHandler == nil {
		applicationHandler = http.DefaultServeMux
	}

	// Get health check path from HealthModule
	healthCheckPath := "/healthz" // default
	if healthModule, ok := s.GetModule(ModuleTypeHealth).(*HealthModule); ok {
		healthCheckPath = healthModule.HealthCheckPath()
	}

	mux.HandleFunc(healthCheckPath, s.HandleHealth)
	mux.Handle("/", applicationHandler)
	return mux
}

func (s *serviceImpl) applyCORSIfEnabled(_ context.Context, muxToWrap http.Handler) http.Handler {
	config, ok := s.Config().(ConfigurationCORS)
	if ok && config.IsCORSEnabled() {
		corsOptions := []ghandler.CORSOption{
			ghandler.AllowedHeaders(config.GetCORSAllowedHeaders()),
			ghandler.ExposedHeaders(config.GetCORSExposedHeaders()),
			ghandler.AllowedOrigins(config.GetCORSAllowedOrigins()),
			ghandler.AllowedMethods(config.GetCORSAllowedMethods()),
			ghandler.MaxAge(config.GetCORSMaxAge()),
		}

		if config.IsCORSAllowCredentials() {
			corsOptions = append(corsOptions, ghandler.AllowCredentials())
		}
		return ghandler.CORS(corsOptions...)(muxToWrap)
	}
	return muxToWrap
}

func (s *serviceImpl) initializeServerDrivers(ctx context.Context, httpPort string) {
	// Get handler from ServerModule
	var handler http.Handler
	if serverModule, ok := s.GetModule(ModuleTypeServer).(*ServerModule); ok {
		handler = serverModule.Handler()
	}
	
	defaultServer := struct {
		ctx  context.Context
		log  *util.LogEntry
		port string
		httpServer *http.Server
	}{
		ctx:  ctx,
		log:  s.Log(ctx),
		port: httpPort,
		httpServer: &http.Server{
			Handler: handler, // handler from ServerModule
			BaseContext: func(_ net.Listener) context.Context {
				return ctx
			},
			ReadTimeout:  defaultHTTPReadTimeoutSeconds * time.Second,
			WriteTimeout: defaultHTTPWriteTimeoutSeconds * time.Second,
			IdleTimeout:  defaultHTTPIdleTimeoutSeconds * time.Second,
		},
	}

	// If grpc server is setup, configure the grpcDriver.
	// Always add the gRPC driver if it's configured.
	if serverModule, ok := s.GetModule(ModuleTypeServer).(*ServerModule); ok {
		grpcServer := serverModule.GRPCServer()
		if grpcServer != nil {
			// TODO: Implement proper gRPC health server
			// grpcHS := NewGrpcHealthServer(s)
			// grpc_health_v1.RegisterHealthServer(grpcServer, grpcHS)

			if serverModule.GRPCServerEnableReflection() {
				reflection.Register(grpcServer)
			}

			// TODO: Implement proper gRPC driver
			// grpcDriverInstance := &grpcDriver{
			//	defaultDriver: defaultServer,
			//	grpcPort:      serverModule.GRPCPort(),
			//	grpcServer:    grpcServer,
			//	grpcListener:  serverModule.secListener,
			// }
			
			// Update ServerModule with the driver
			*serverModule = *NewServerModule(
				serverModule.ServerManager(),
				serverModule.Handler(),
				grpcServer,
				serverModule.GRPCServerEnableReflection(),
				serverModule.priListener,
				serverModule.secListener,
				serverModule.GRPCPort(),
				nil, // TODO: pass proper driver when implemented
			)
		} else {
			// Update ServerModule with default driver
			*serverModule = *NewServerModule(
				serverModule.ServerManager(),
				serverModule.Handler(),
				serverModule.GRPCServer(),
				serverModule.GRPCServerEnableReflection(),
				serverModule.priListener,
				serverModule.secListener,
				serverModule.GRPCPort(),
				&defaultServer,
			)
		}
	}
}

// initServer starts the Service. It initializes server drivers (HTTP, gRPC).
func (s *serviceImpl) initServer(ctx context.Context, httpPort string) error {
	// TODO: Implement proper tracer initialization
	// err := s.initTracer(ctx)
	// if err != nil {
	//	return err
	// }

	// Health check path and handler are now managed by modules
	healthModule, hasHealth := s.GetModule(ModuleTypeHealth).(*HealthModule)
	serverModule, hasServer := s.GetModule(ModuleTypeServer).(*ServerModule)
	
	if hasHealth && hasServer {
		healthCheckPath := healthModule.HealthCheckPath()
		handler := serverModule.Handler()
		
		if healthCheckPath == "" || (healthCheckPath == "/" && handler != nil) {
			// Update health module with default path
			*healthModule = *NewHealthModule(healthModule.HealthCheckers(), "/healthz")
		}
	}

	httpPort = s.determineHTTPPort(httpPort)

	// Update gRPC port via ServerModule if gRPC server exists
	if serverModule, ok := s.GetModule(ModuleTypeServer).(*ServerModule); ok {
		if serverModule.GRPCServer() != nil {
			grpcPort := s.determineGRPCPort(serverModule.GRPCPort())
			// Update ServerModule with new gRPC port
			*serverModule = *NewServerModule(
				serverModule.ServerManager(),
				serverModule.Handler(),
				serverModule.GRPCServer(),
				serverModule.GRPCServerEnableReflection(),
				serverModule.priListener,
				serverModule.secListener,
				grpcPort,
				serverModule.driver,
			)
		}
	}

	s.startOnce.Do(func() {
		baseMux := s.createAndConfigureMux(ctx)
		corsHandler := s.applyCORSIfEnabled(ctx, baseMux)
		
		// Update ServerModule with handler
		if serverModule, ok := s.GetModule(ModuleTypeServer).(*ServerModule); ok {
			*serverModule = *NewServerModule(
				serverModule.ServerManager(),
				corsHandler,
				serverModule.GRPCServer(),
				serverModule.GRPCServerEnableReflection(),
				serverModule.priListener,
				serverModule.secListener,
				serverModule.GRPCPort(),
				serverModule.driver,
			)
		}
		
		s.initializeServerDrivers(ctx, httpPort)
	})

	if s.startup != nil {
		s.startup(ctx, s)
	}

	// TODO: Implement proper TLS check and configuration
	/*
	if s.TLSEnabled() {
		config, ok := s.Config().(ConfigurationTLS)
		if !ok {
			return errors.New("TLS is enabled but configuration does not implement ConfigurationTLS")
		}
		// Get driver and handler from ServerModule
		if serverModule, ok := s.GetModule(ModuleTypeServer).(*ServerModule); ok {
			driver := serverModule.driver
			handler := serverModule.Handler()
			
			tlsServer, ok := driver.(internal.TLSServer)
		}
	}
	*/

	// Get driver and handler from ServerModule for non-TLS mode
	if serverModule, ok := s.GetModule(ModuleTypeServer).(*ServerModule); ok {
		driver := serverModule.driver
		handler := serverModule.Handler()
		
		nonTLSServer, ok := driver.(internal.Server)
		if !ok {
			return errors.New("driver does not implement internal.Server for non-TLS mode")
		}
		return nonTLSServer.ListenAndServe(httpPort, handler)
	}
	return errors.New("ServerModule not found")
}

// Stop Used to gracefully run clean up methods ensuring all requests that
// were being handled are completed well without interuptions.
func (s *serviceImpl) Stop(ctx context.Context) {
	if !s.stopMutex.TryLock() {
		return
	}
	defer s.stopMutex.Unlock()

	s.Log(ctx).Info("service stopping")

	// Cancel the service's main context.
	if s.cancelFunc != nil {
		s.logger.Info("canceling service context")
		s.cancelFunc()
	}

	// Call user-defined cleanup functions first.
	if s.cleanup != nil {
		s.cleanup(ctx)
	}

	// Release the worker pool via WorkerPoolModule.
	if workerPoolModule, ok := s.GetModule(ModuleTypeWorkerPool).(*WorkerPoolModule); ok {
		pool := workerPoolModule.Pool()
		if pool != nil {
			s.logger.Info("shutting down worker pool")
			// TODO: Implement proper pool shutdown with type assertion
			// if shutdownable, ok := pool.(interface{ Shutdown() }); ok {
			//	shutdownable.Shutdown()
			// }
		}
	}

	// Close the internal error channel to signal Run to exit if it's blocked on it.
	s.errorChannelMutex.Lock()
	select {
	case _, ok := <-s.errorChannel:
		if !ok {
			return
		}
	default:
	}
	close(s.errorChannel)
	defer s.errorChannelMutex.Unlock()
}

func (s *serviceImpl) sendStopError(ctx context.Context, err error) {
	s.errorChannelMutex.Lock()
	defer s.errorChannelMutex.Unlock()

	select {
	case <-ctx.Done():
		return
	case <-s.errorChannel:
		// channel is already closed hence avoid
		return
	default:
		s.errorChannel <- err
	}
}

// TLSEnabled checks if the service is configured to run with TLS.
