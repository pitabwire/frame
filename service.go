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
	"github.com/pitabwire/frame/core"
	"github.com/pitabwire/util"
	"gorm.io/gorm"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	_ "go.uber.org/automaxprocs" // Automatically set GOMAXPROCS to match Linux container CPU quota.
	"google.golang.org/grpc/reflection"
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
	startup func(ctx context.Context, s core.Service)
	cleanup func(ctx context.Context)
	
	// Module registry for plugin management
	moduleRegistry *core.ModuleRegistry
}

// Note: Option type is now defined in interface.go

// NewService creates a new instance of Service with the name and supplied options.
// Internally it calls NewServiceWithContext and creates a background context for use.
func NewService(name string, opts ...core.Option) (context.Context, core.Service) {
	ctx := context.Background()
	return NewServiceWithContext(ctx, name, opts...)
}

// NewServiceWithContext creates a new instance of Service with context, name and supplied options
// It is used together with the Init option to setup components of a service that is not yet running.
func NewServiceWithContext(ctx context.Context, name string, opts ...core.Option) (context.Context, core.Service) {
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
		moduleRegistry: core.NewModuleRegistry(),
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
func SvcToContext(ctx context.Context, service core.Service) context.Context {
	return context.WithValue(ctx, ctxKeyService, service)
}

// Svc obtains a service instance being propagated through the context.
func Svc(ctx context.Context) core.Service {
	service, ok := ctx.Value(ctxKeyService).(core.Service)
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
func WithName(name string) core.Option {
	return func(_ context.Context, s core.Service) {
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
func WithVersion(version string) core.Option {
	return func(_ context.Context, s core.Service) {
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
func WithEnvironment(environment string) core.Option {
	return func(_ context.Context, s core.Service) {
		if impl, ok := s.(*serviceImpl); ok {
			impl.environment = environment
		}
	}
}

// Config gets the configuration object associated with the service.
func (s *serviceImpl) Config() any {
	return s.configuration
}

// Log method is now implemented in logger.go using LoggingModule

// Init evaluates the options provided as arguments and supplies them to the service object.
func (s *serviceImpl) Init(ctx context.Context, opts ...core.Option) {
	for _, opt := range opts {
		opt(ctx, s)
	}
}

// AddPreStartMethod Adds user defined functions that can be run just before
// the service starts receiving requests but is fully initialized.
func (s *serviceImpl) AddPreStartMethod(f func(ctx context.Context, s core.Service)) {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()
	if s.startup == nil {
		s.startup = f
		return
	}

	old := s.startup
	s.startup = func(ctx context.Context, st core.Service) { old(ctx, st); f(ctx, st) }
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

// ModuleService interface methods
func (s *serviceImpl) Modules() *core.ModuleRegistry {
	return s.moduleRegistry
}

func (s *serviceImpl) GetModule(moduleType core.ModuleType) core.Module {
	return s.moduleRegistry.Get(moduleType)
}

func (s *serviceImpl) GetTypedModule(moduleType core.ModuleType, target interface{}) bool {
	return s.moduleRegistry.GetTyped(moduleType, target)
}

func (s *serviceImpl) RegisterModule(module core.Module) error {
	return s.moduleRegistry.Register(module)
}

func (s *serviceImpl) HasModule(moduleType core.ModuleType) bool {
	module := s.moduleRegistry.Get(moduleType)
	return module != nil && module.IsEnabled()
}

// Run keeps the service useful by handling incoming requests.
func (s *serviceImpl) Run(ctx context.Context, address string) error {
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

	// Configuration for ports should be handled by the server module.
	return ":8080" // Default HTTP port
}

func (s *serviceImpl) determineGRPCPort(currentPort string) string {
	if currentPort != "" {
		return currentPort
	}

	// Configuration for ports should be handled by the server module.
	return ":50051" // Default gRPC port
}

func (s *serviceImpl) createAndConfigureMux(_ context.Context) *http.ServeMux {
	mux := http.NewServeMux()

	// The server module should be responsible for handling routes.

	return mux
}

func (s *serviceImpl) applyCORSIfEnabled(_ context.Context, muxToWrap http.Handler) http.Handler {
	// CORS configuration should be handled by the server module.
	return muxToWrap
}

func (s *serviceImpl) initializeServerDrivers(ctx context.Context, httpPort string) {
	// Server driver initialization should be handled by the server module.
}

// initServer starts the Service. It initializes server drivers (HTTP, gRPC).
func (s *serviceImpl) initServer(ctx context.Context, httpPort string) error {
	// Server initialization should be handled by the server module.
	return nil
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

	// Stop all modules.
	s.moduleRegistry.Stop(ctx)

	// Close the internal error channel to signal Run to exit if it's blocked on it.
	s.errorChannelMutex.Lock()
	select {
	case <-ctx.Done():
		return
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
