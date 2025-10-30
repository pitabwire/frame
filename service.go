package frame

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pitabwire/util"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gocloud.dev/server/driver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/events"
	"github.com/pitabwire/frame/localization"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/frame/security"
	securityManager "github.com/pitabwire/frame/security/manager"
	"github.com/pitabwire/frame/telemetry"
	"github.com/pitabwire/frame/workerpool"
)

type contextKey string

func (c contextKey) String() string {
	return "frame/" + string(c)
}

const (
	ctxKeyService = contextKey("serviceKey")

	defaultHTTPReadTimeoutSeconds  = 5
	defaultHTTPWriteTimeoutSeconds = 10
	defaultHTTPTimeoutSeconds      = 30
	defaultHTTPIdleTimeoutSeconds  = 90
)

// Service framework struct to hold together all application components
// An instance of this type scoped to stay for the lifetime of the application.
// It is pushed and pulled from contexts to make it easy to pass around.
type Service struct {
	name string

	version     string
	environment string
	logger      *util.LogEntry

	handler    http.Handler
	cancelFunc context.CancelFunc

	errorChannelMutex sync.Mutex
	errorChannel      chan error

	backGroundClient func(ctx context.Context) error

	driver                     ServerDriver
	grpcServer                 *grpc.Server
	grpcServerEnableReflection bool
	grpcListener               net.Listener
	grpcPort                   string

	healthCheckers  []Checker
	healthCheckPath string
	startup         func(ctx context.Context, s *Service)
	cleanup         func(ctx context.Context)

	configuration any

	clientManager       client.Manager
	workerPoolManager   workerpool.Manager
	localizationManager localization.Manager
	securityManager     security.Manager
	cacheManager        cache.Manager
	queueManager        queue.Manager
	eventsManager       events.Manager
	datastoreManager    datastore.Manager

	telemetryManager telemetry.Manager

	startOnce            sync.Once
	startupOnce          sync.Once
	startupCompleted     bool
	stopMutex            sync.Mutex
	startupErrors        []error
	startupMutex         sync.Mutex
	publisherStartups    []func(ctx context.Context, s *Service)
	subscriberStartups   []func(ctx context.Context, s *Service)
	otherStartups        []func(ctx context.Context, s *Service)
	startupRegistrations sync.Mutex
}

type Option func(ctx context.Context, service *Service)

// NewService creates a new instance of Service with the name and supplied options.
// Internally it calls NewServiceWithContext and creates a background context for use.
func NewService(name string, opts ...Option) (context.Context, *Service) {
	ctx := context.Background()
	return NewServiceWithContext(ctx, name, opts...)
}

// NewServiceWithContext creates a new instance of Service with context, name and supplied options
// It is used together with the Init option to setup components of a service that is not yet running.
func NewServiceWithContext(ctx context.Context, name string, opts ...Option) (context.Context, *Service) {
	// Create a new context that listens for OS signals for graceful shutdown.
	ctx, signalCancelFunc := signal.NotifyContext(ctx,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	var err error
	log := util.Log(ctx)
	ctx = util.ContextWithLogger(ctx, log)

	cfg, _ := config.FromEnv[config.ConfigurationDefault]()

	svc := &Service{
		name:         name,
		cancelFunc:   signalCancelFunc, // Store its cancel function
		errorChannel: make(chan error, 1),

		logger: log,

		configuration: &cfg,
		clientManager: client.NewManager(&cfg),
	}

	if cfg.ServiceName != "" {
		opts = append(opts, WithName(cfg.ServiceName))
	}

	if cfg.ServiceEnvironment != "" {
		opts = append(opts, WithEnvironment(cfg.ServiceEnvironment))
	}

	if cfg.ServiceVersion != "" {
		opts = append(opts, WithVersion(cfg.ServiceVersion))
	}

	var telemetryOpts []telemetry.Option
	if cfg.OpenTelemetryDisable {
		telemetryOpts = append(telemetryOpts, telemetry.WithDisableTracing())
	}

	opts = append(opts, WithLogger()) // Ensure logger is initialized early

	opts = append(opts, WithTelemetry(telemetryOpts...))

	svc.Init(ctx, opts...) // Apply all options, using the signal-aware context

	// Create security manager AFTER options are applied so it gets the correct config
	finalCfg, ok := svc.Config().(*config.ConfigurationDefault)
	if !ok {
		finalCfg = &cfg
	}
	svc.securityManager = securityManager.NewManager(ctx, finalCfg, svc.clientManager)

	svc.workerPoolManager = workerpool.NewManager(ctx, &cfg, svc.sendStopError)

	svc.queueManager = queue.NewQueueManager(ctx, svc.workerPoolManager)

	// Setup events queue first (before startup methods)
	// This registers the internal events publisher/subscriber
	err = svc.setupEventsQueue(ctx)
	if err != nil {
		log.WithError(err).Panic("could not setup application events")
	}

	// Execute pre-start methods now that queue manager is initialized
	// This ensures queue registrations from options are applied
	// Run in order: publishers first, then subscribers, then other startups
	svc.startupOnce.Do(func() {
		// Run publisher startups first (to create topics for mem:// driver)
		for _, startup := range svc.publisherStartups {
			startup(ctx, svc)
		}
		// Run subscriber startups after publishers
		for _, startup := range svc.subscriberStartups {
			startup(ctx, svc)
		}
		// Run other startups last
		for _, startup := range svc.otherStartups {
			startup(ctx, svc)
		}
		// Run legacy startup function if set
		if svc.startup != nil {
			svc.startup(ctx, svc)
		}
		// Mark startup as completed
		svc.startupRegistrations.Lock()
		svc.startupCompleted = true
		svc.startupRegistrations.Unlock()
	})

	// Prepare context to be returned, embedding svc and config
	ctx = ToContext(ctx, svc)
	ctx = config.ToContext(ctx, svc.Config())
	ctx = util.ContextWithLogger(ctx, log)
	return ctx, svc
}

// ToContext pushes a service instance into the supplied context for easier propagation.
func ToContext(ctx context.Context, service *Service) context.Context {
	return context.WithValue(ctx, ctxKeyService, service)
}

// FromContext obtains a service instance being propagated through the context.
func FromContext(ctx context.Context) *Service {
	service, ok := ctx.Value(ctxKeyService).(*Service)
	if !ok {
		return nil
	}

	return service
}

// Name gets the name of the service. Its the first argument used when NewService is called.
func (s *Service) Name() string {
	return s.name
}

// WithName specifies the name the service will utilize.
func WithName(name string) Option {
	return func(_ context.Context, s *Service) {
		s.name = name
	}
}

// Version gets the release version of the service.
func (s *Service) Version() string {
	return s.version
}

// WithVersion specifies the version the service will utilize.
func WithVersion(version string) Option {
	return func(_ context.Context, s *Service) {
		s.version = version
	}
}

// Environment gets the runtime environment of the service.
func (s *Service) Environment() string {
	return s.environment
}

// WithEnvironment specifies the environment the service will utilize.
func WithEnvironment(environment string) Option {
	return func(_ context.Context, s *Service) {
		s.environment = environment
	}
}

func (s *Service) H() http.Handler {
	return s.handler
}

// Init evaluates the options provided as arguments and supplies them to the service object.
// If called after initial startup, it will execute any new startup methods immediately.
func (s *Service) Init(ctx context.Context, opts ...Option) {
	s.startupRegistrations.Lock()
	initialPublisherCount := len(s.publisherStartups)
	initialSubscriberCount := len(s.subscriberStartups)
	initialOtherCount := len(s.otherStartups)
	alreadyStarted := s.startupCompleted
	s.startupRegistrations.Unlock()

	for _, opt := range opts {
		opt(ctx, s)
	}

	// If startup has already run, execute new startup methods immediately
	if alreadyStarted {
		s.startupRegistrations.Lock()
		newPublishers := s.publisherStartups[initialPublisherCount:]
		newSubscribers := s.subscriberStartups[initialSubscriberCount:]
		newOthers := s.otherStartups[initialOtherCount:]
		s.startupRegistrations.Unlock()

		// Run new startup methods in order
		for _, startup := range newPublishers {
			startup(ctx, s)
		}
		for _, startup := range newSubscribers {
			startup(ctx, s)
		}
		for _, startup := range newOthers {
			startup(ctx, s)
		}
	}
}

// AddPreStartMethod Adds user defined functions that can be run just before
// the service starts receiving requests but is fully initialized.
func (s *Service) AddPreStartMethod(f func(ctx context.Context, s *Service)) {
	s.startupRegistrations.Lock()
	defer s.startupRegistrations.Unlock()
	s.otherStartups = append(s.otherStartups, f)
}

// AddPublisherStartup Adds publisher initialization functions that run before subscribers.
func (s *Service) AddPublisherStartup(f func(ctx context.Context, s *Service)) {
	s.startupRegistrations.Lock()
	defer s.startupRegistrations.Unlock()
	s.publisherStartups = append(s.publisherStartups, f)
}

// AddSubscriberStartup Adds subscriber initialization functions that run after publishers.
func (s *Service) AddSubscriberStartup(f func(ctx context.Context, s *Service)) {
	s.startupRegistrations.Lock()
	defer s.startupRegistrations.Unlock()
	s.subscriberStartups = append(s.subscriberStartups, f)
}

// AddStartupError stores errors that occur during startup initialization.
func (s *Service) AddStartupError(err error) {
	if err == nil {
		return
	}
	s.startupMutex.Lock()
	defer s.startupMutex.Unlock()
	s.startupErrors = append(s.startupErrors, err)
}

// GetStartupErrors returns all errors that occurred during startup.
func (s *Service) GetStartupErrors() []error {
	s.startupMutex.Lock()
	defer s.startupMutex.Unlock()
	if len(s.startupErrors) == 0 {
		return nil
	}
	// Return a copy to prevent external modification
	errorsCopy := make([]error, len(s.startupErrors))
	copy(errorsCopy, s.startupErrors)
	return errorsCopy
}

// AddCleanupMethod Adds user defined functions to be run just before completely stopping the service.
// These are responsible for properly and gracefully stopping active components.
func (s *Service) AddCleanupMethod(f func(ctx context.Context)) {
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
func (s *Service) AddHealthCheck(checker Checker) {
	if s.healthCheckers != nil {
		s.healthCheckers = []Checker{}
	}
	s.healthCheckers = append(s.healthCheckers, checker)
}

// Run keeps the service useful by handling incoming requests.
func (s *Service) Run(ctx context.Context, address string) error {
	// Check for any errors that occurred during startup initialization
	if startupErrs := s.GetStartupErrors(); len(startupErrs) > 0 {
		return startupErrs[0] // Return the first error
	}

	pubSubErr := s.queueManager.Init(ctx)
	if pubSubErr != nil {
		return pubSubErr
	}

	// connect the background processor
	if s.backGroundClient != nil {
		go func() {
			bgErr := s.backGroundClient(ctx)
			s.sendStopError(ctx, bgErr)
		}()
	}

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

func (s *Service) determineHTTPPort(currentPort string) string {
	if currentPort != "" {
		return currentPort
	}

	cfg, ok := s.Config().(config.ConfigurationPorts)
	if !ok {
		// Assuming s.TLSEnabled() checks if TLS cert/key paths are configured.
		// This part might need adjustment if s.TLSEnabled() is not available
		// or if direct check on ConfigurationTLS is preferred.
		tlsCfg, tlsOk := s.Config().(config.ConfigurationTLS)
		if tlsOk && tlsCfg.TLSCertPath() != "" && tlsCfg.TLSCertKeyPath() != "" {
			return ":https"
		}
		return "http" // Existing logic; consider ":http" or a default port like ":8080"
	}
	return cfg.HTTPPort()
}

func (s *Service) determineGRPCPort(currentPort string) string {
	if currentPort != "" {
		return currentPort
	}

	cfg, ok := s.Config().(config.ConfigurationPorts)
	if !ok {
		return ":50051" // Default gRPC port
	}
	return cfg.GrpcPort()
}

func (s *Service) createAndConfigureMux(_ context.Context) *http.ServeMux {
	mux := http.NewServeMux()

	applicationHandler := s.handler
	if applicationHandler == nil {
		applicationHandler = http.DefaultServeMux
	}

	mux.HandleFunc(s.healthCheckPath, s.HandleHealth)
	mux.Handle("/", applicationHandler)
	return mux
}

func (s *Service) initializeServerDrivers(ctx context.Context, httpPort string) {
	if s.driver == nil {
		s.driver = &defaultDriver{
			ctx:  ctx,
			port: httpPort,
			httpServer: &http.Server{
				Handler: s.handler, // s.handlers is the (potentially CORS-wrapped) mux
				BaseContext: func(_ net.Listener) context.Context {
					return ctx
				},
				ReadTimeout:  defaultHTTPReadTimeoutSeconds * time.Second,
				WriteTimeout: defaultHTTPWriteTimeoutSeconds * time.Second,
				IdleTimeout:  defaultHTTPIdleTimeoutSeconds * time.Second,
			},
		}
	}

	// If grpc server is setup, configure the grpcDriver.
	// Always add the gRPC driver if it's configured.
	if s.grpcServer != nil {
		grpcHS := NewGrpcHealthServer(s)
		grpc_health_v1.RegisterHealthServer(s.grpcServer, grpcHS)

		if s.grpcServerEnableReflection {
			reflection.Register(s.grpcServer)
		}

		s.driver = &grpcDriver{
			ctx:                ctx,
			internalHTTPDriver: s.driver, // Embed the fully configured defaultServer
			grpcPort:           s.grpcPort,
			grpcServer:         s.grpcServer,
			grpcListener:       s.grpcListener, // Use the primary listener established for gRPC
		}
	}
}

// executeStartupMethods runs all startup methods in the correct order.
func (s *Service) executeStartupMethods(ctx context.Context) {
	s.startupOnce.Do(func() {
		// Run publisher startups first (to create topics for mem:// driver)
		for _, startup := range s.publisherStartups {
			startup(ctx, s)
		}
		// Run subscriber startups after publishers
		for _, startup := range s.subscriberStartups {
			startup(ctx, s)
		}
		// Run other startups last
		for _, startup := range s.otherStartups {
			startup(ctx, s)
		}
		// Run legacy startup function if set
		if s.startup != nil {
			s.startup(ctx, s)
		}
		// Mark startup as completed
		s.startupRegistrations.Lock()
		s.startupCompleted = true
		s.startupRegistrations.Unlock()
	})
}

// startServerDriver starts either TLS or non-TLS server based on configuration.
func (s *Service) startServerDriver(httpPort string) error {
	if s.TLSEnabled() {
		cfg, ok := s.Config().(config.ConfigurationTLS)
		if !ok {
			return errors.New("TLS is enabled but configuration does not implement ConfigurationTLS")
		}
		tlsServer, ok := s.driver.(driver.TLSServer)
		if !ok {
			return errors.New("driver does not implement internal.TLSServer for TLS mode")
		}
		return tlsServer.ListenAndServeTLS(httpPort, cfg.TLSCertPath(), cfg.TLSCertKeyPath(), s.handler)
	}

	nonTLSServer, ok := s.driver.(driver.Server)
	if !ok {
		return errors.New("driver does not implement internal.Server for non-TLS mode")
	}
	return nonTLSServer.ListenAndServe(httpPort, s.handler)
}

// initServer starts the Service. It initializes server drivers (HTTP, gRPC).
func (s *Service) initServer(ctx context.Context, httpPort string) error {
	if s.healthCheckPath == "" ||
		(s.healthCheckPath == "/" && s.handler != nil) {
		s.healthCheckPath = "/healthz"
	}

	httpPort = s.determineHTTPPort(httpPort)

	if s.grpcServer != nil {
		s.grpcPort = s.determineGRPCPort(s.grpcPort)
	}

	s.startOnce.Do(func() {
		rootHandler := s.createAndConfigureMux(ctx)
		if s.telemetryManager.Disabled() {
			s.handler = rootHandler
		} else {
			s.handler = otelhttp.NewHandler(rootHandler, s.Name())
		}
		s.initializeServerDrivers(ctx, httpPort)
	})

	// Execute pre-start methods
	s.executeStartupMethods(ctx)

	return s.startServerDriver(httpPort)
}

// Stop Used to gracefully run clean up methods ensuring all requests that
// were being handled are completed well without interuptions.
func (s *Service) Stop(ctx context.Context) {
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

func (s *Service) sendStopError(ctx context.Context, err error) {
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
