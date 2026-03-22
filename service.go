package frame

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pitabwire/util"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/events"
	"github.com/pitabwire/frame/localization"
	"github.com/pitabwire/frame/openapi"
	"github.com/pitabwire/frame/profiler"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/frame/security"
	httpInterceptor "github.com/pitabwire/frame/security/interceptors/httptor"
	securityManager "github.com/pitabwire/frame/security/manager"
	"github.com/pitabwire/frame/server"
	"github.com/pitabwire/frame/server/implementation"
	"github.com/pitabwire/frame/telemetry"
	"github.com/pitabwire/frame/version"
	"github.com/pitabwire/frame/workerpool"
)

type contextKey string

func (c contextKey) String() string {
	return "frame/" + string(c)
}

const (
	ctxKeyService = contextKey("serviceKey")

	defaultOpenAPIBasePath = "/debug/frame/openapi"
)

var ErrHTTPServerConfigRequired = errors.New("configuration must implement config.ConfigurationHTTPServer")

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
	httpMW     []func(http.Handler) http.Handler

	errorChannelMutex sync.Mutex
	errorChannel      chan error

	backGroundClient func(ctx context.Context) error

	driver server.Driver

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

	profilerServer *profiler.Server

	openapiRegistry   *openapi.Registry
	openapiBasePath   string
	debugEnabled      bool
	debugBasePath     string
	registeredPlugins []string
	routeLister       RouteLister

	startedAt            time.Time
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
func NewService(opts ...Option) (context.Context, *Service) {
	ctx := context.Background()
	return NewServiceWithContext(ctx, opts...)
}

// NewServiceWithContext creates a new instance of Service with context, name and supplied options
// It is used together with the Init option to setup components of a service that is not yet running.
func NewServiceWithContext(ctx context.Context, opts ...Option) (context.Context, *Service) {
	// Create a new context that listens for OS signals for graceful shutdown.
	ctx, signalCancelFunc := signal.NotifyContext(ctx,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	cfg, _ := config.FromEnv[config.ConfigurationDefault]()

	svc := &Service{
		name:            cfg.Name(),
		cancelFunc:      signalCancelFunc, // Store its cancel function
		errorChannel:    make(chan error, 1),
		configuration:   &cfg,
		profilerServer:  profiler.NewServer(),
		openapiBasePath: defaultOpenAPIBasePath,
	}

	if dbgCfg, ok := any(&cfg).(config.ConfigurationDebug); ok {
		if dbgCfg.DebugEndpointsEnabled() {
			svc.debugEnabled = true
			svc.debugBasePath = dbgCfg.DebugEndpointsBasePath()
		}
	}

	opts = append(
		[]Option{WithTelemetry(), WithLogger(), WithHTTPClient()},
		opts...) // Ensure prerequisites are initialized early

	svc.Init(ctx, opts...) // Apply all options, using the signal-aware context

	// Create security manager AFTER options are applied so it gets the correct config
	smCfg, ok := svc.Config().(securityManager.SecurityConfiguration)
	if !ok {
		smCfg = &cfg
	}
	svc.securityManager = securityManager.NewManager(ctx, svc.name, svc.environment, smCfg, svc.clientManager)
	svc.registerPlugin("security")
	// Register cleanup for the security manager to stop background goroutines (e.g., JWKS refresh)
	svc.AddCleanupMethod(func(_ context.Context) {
		if svc.securityManager != nil {
			svc.securityManager.Close()
		}
	})

	svc.initWorkersAndQueues(ctx, &cfg)

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
	// Put the raw logger into context, not the context-aware one
	ctx = util.ContextWithLogger(ctx, svc.logger)

	return ctx, svc
}

// ToContext pushes a service instance into the supplied context for easier propagation.
func ToContext(ctx context.Context, service *Service) context.Context {
	return context.WithValue(ctx, ctxKeyService, service)
}

// FromContext obtains a service instance being propagated through the context.
func (s *Service) registerPlugin(name string) {
	if name == "" {
		return
	}
	if slices.Contains(s.registeredPlugins, name) {
		return
	}
	s.registeredPlugins = append(s.registeredPlugins, name)
}

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
	if s.healthCheckers == nil {
		s.healthCheckers = []Checker{}
	}
	s.healthCheckers = append(s.healthCheckers, checker)
}

// Run keeps the service useful by handling incoming requests.
func (s *Service) Run(ctx context.Context, address string) error {
	s.startedAt = time.Now()
	log := util.Log(ctx)
	log.WithFields(map[string]any{
		"repository": version.Repository,
		"version":    version.Version,
		"commit":     version.Commit,
		"date":       version.Date,
	}).Info("Build info")

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
		s.stopWithTimeout(ctx)
		return ctx.Err()
	case err0 := <-s.errorChannel:
		if err0 != nil {
			s.Log(ctx).
				WithError(err0).
				Error("system exit in error")
			s.stopWithTimeout(ctx)
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
		return ":http" // Existing logic; consider ":http" or a default port like ":8080"
	}
	return cfg.HTTPPort()
}

func (s *Service) createAndConfigureMux(ctx context.Context) *http.ServeMux {
	applicationHandler := s.handler
	if applicationHandler == nil {
		applicationHandler = http.DefaultServeMux
	}

	if len(s.httpMW) > 0 {
		for i := len(s.httpMW) - 1; i >= 0; i-- {
			if s.httpMW[i] == nil {
				continue
			}
			applicationHandler = s.httpMW[i](applicationHandler)
		}
	}

	cfg, ok := s.Config().(config.ConfigurationTraceRequests)
	if ok {
		if cfg.TraceReq() {
			applicationHandler = httpInterceptor.LoggingMiddleware(applicationHandler, cfg.TraceReqLogBody())
		}
	}

	applicationHandler = httpInterceptor.ContextSetupMiddleware(ctx, applicationHandler)

	mux := http.NewServeMux()

	s.registerDebugEndpoints(mux)
	mux.HandleFunc(s.healthCheckPath, s.HandleHealth)
	s.registerOpenAPIRoutes(mux)
	s.registerOAuth2ClientJWKSRoute(mux)
	mux.Handle("/", applicationHandler)
	return mux
}

func (s *Service) registerOpenAPIRoutes(mux *http.ServeMux) {
	if s.openapiRegistry == nil {
		return
	}
	if len(s.openapiRegistry.List()) == 0 {
		return
	}
	base := s.openapiBasePath
	if base == "" {
		base = defaultOpenAPIBasePath
	}
	handler := s.openAPIHandler(base)
	mux.Handle(base, handler)
	mux.Handle(base+"/", handler)
}

func (s *Service) openAPIHandler(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == base || r.URL.Path == base+"/" {
			openapi.ServeIndex(s.openapiRegistry)(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, base+"/") {
			req := r.Clone(r.Context())
			req.URL.Path = strings.TrimPrefix(r.URL.Path, base+"/")
			openapi.ServeSpec(s.openapiRegistry)(w, req)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *Service) initializeServerDrivers(ctx context.Context, httpPort string) error {
	if s.driver != nil {
		return nil
	}

	httpCfg := s.mustHTTPServerConfig()
	if httpCfg == nil {
		return ErrHTTPServerConfigRequired
	}

	tlsConfig, ok, err := s.setupWorkloadAPITLS(ctx)
	if err != nil {
		return err
	}
	if !ok {
		tlsConfig = nil
	}

	s.driver = implementation.NewDefaultDriverWithTLS(
		ctx,
		httpCfg,
		s.handler,
		httpPort,
		tlsConfig,
	)

	return nil
}

func (s *Service) setupWorkloadAPITLS(ctx context.Context) (*tls.Config, bool, error) {
	if !s.shouldUseWorkloadAPIServerTLS() {
		return nil, false, nil
	}

	if s.securityManager == nil {
		return nil, false, nil
	}

	workloadAPI := s.securityManager.GetWorkloadAPI(ctx)
	if workloadAPI == nil {
		return nil, false, nil
	}

	tlsConfig, err := workloadAPI.Setup(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("secure transport setup failed: %w", err)
	}

	return tlsConfig, true, nil
}

func (s *Service) shouldUseWorkloadAPIServerTLS() bool {
	securityCfg, ok := s.Config().(config.ConfigurationSecurity)
	if !ok || !securityCfg.IsRunSecurely() || s.TLSEnabled() {
		return false
	}

	workloadCfg, ok := s.Config().(config.ConfigurationWorkloadAPI)
	if !ok {
		return false
	}

	return strings.TrimSpace(workloadCfg.GetTrustedDomain()) != ""
}

func (s *Service) mustHTTPServerConfig() config.ConfigurationHTTPServer {
	cfg, ok := s.Config().(config.ConfigurationHTTPServer)
	if ok {
		return cfg
	}

	return nil
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
func (s *Service) startServerDriver(ctx context.Context, httpPort string) error {
	util.Log(ctx).WithField("port", httpPort).Info("Initiating server operations")

	if s.TLSEnabled() {
		cfg, ok := s.Config().(config.ConfigurationTLS)
		if !ok {
			return errors.New("TLS is enabled but configuration does not implement ConfigurationTLS")
		}
		err := s.driver.ListenAndServeTLS(httpPort, cfg.TLSCertPath(), cfg.TLSCertKeyPath(), s.handler)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}

	err := s.driver.ListenAndServe(httpPort, s.handler)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// initServer starts the Service. It initializes server drivers.
func (s *Service) initServer(ctx context.Context, httpPort string) error {
	if s.healthCheckPath == "" ||
		(s.healthCheckPath == "/" && s.handler != nil) {
		s.healthCheckPath = "/healthz"
	}

	httpPort = s.determineHTTPPort(httpPort)

	s.startOnce.Do(func() {
		rootHandler := s.createAndConfigureMux(ctx)
		if s.telemetryManager.Disabled() {
			s.handler = rootHandler
		} else {
			s.handler = otelhttp.NewHandler(rootHandler, s.Name())
		}
	})

	if err := s.initializeServerDrivers(ctx, httpPort); err != nil {
		return err
	}

	// Start profiler if enabled
	if err := s.startProfilerIfEnabled(ctx); err != nil {
		return err
	}

	// Execute pre-start methods
	s.executeStartupMethods(ctx)

	return s.startServerDriver(ctx, httpPort)
}

// startProfilerIfEnabled checks if profiler is enabled and starts pprof server.
func (s *Service) startProfilerIfEnabled(ctx context.Context) error {
	cfg, ok := s.Config().(config.ConfigurationProfiler)
	if !ok {
		// Configuration doesn't implement profiler interface, assume disabled
		return nil
	}

	return s.profilerServer.StartIfEnabled(ctx, cfg)
}

// Stop Used to gracefully run clean up methods ensuring all requests that
// were being handled are completed well without interuptions.
func (s *Service) Stop(ctx context.Context) {
	if !s.stopMutex.TryLock() {
		return
	}
	defer s.stopMutex.Unlock()

	log := util.Log(ctx)

	log.Info("service stopping")

	if s.driver != nil {
		if err := s.driver.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.WithError(err).Error("failed to shutdown HTTP server")
		}
	}

	// Stop profiler server if it was started
	if s.profilerServer != nil {
		if err := s.profilerServer.Stop(ctx); err != nil {
			log.WithError(err).Error("failed to shutdown pprof server")
		}
	}

	// Call user-defined cleanup functions first.
	if s.cleanup != nil {
		s.cleanup(ctx)
	}

	// Cancel the service's main context after listeners and cleanup paths have drained.
	if s.cancelFunc != nil {
		log.Info("canceling service context")
		s.cancelFunc()
	}
}

func (s *Service) stopWithTimeout(ctx context.Context) {
	shutdownCtx, cancel := s.shutdownContext(ctx)
	defer cancel()
	s.Stop(shutdownCtx)
}

func (s *Service) shutdownContext(_ context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	base = ToContext(base, s)
	base = config.ToContext(base, s.Config())
	base = util.ContextWithLogger(base, s.logger)
	httpCfg := s.mustHTTPServerConfig()
	if httpCfg == nil {
		return base, func() {}
	}

	ctx, cancel := context.WithTimeout(base, httpCfg.HTTPShutdownTimeout())
	return ctx, cancel
}

// initWorkersAndQueues sets up the worker pool manager, queue manager, and events queue.
func (s *Service) initWorkersAndQueues(ctx context.Context, cfg *config.ConfigurationDefault) {
	wkpCfg, ok := s.Config().(config.ConfigurationWorkerPool)
	if !ok {
		wkpCfg = cfg
	}
	var err error
	s.workerPoolManager, err = workerpool.NewManager(ctx, wkpCfg, s.sendStopError)
	if err != nil {
		s.AddStartupError(err)
	}
	s.AddCleanupMethod(func(cleanupCtx context.Context) {
		if s.workerPoolManager != nil {
			_ = s.workerPoolManager.Shutdown(cleanupCtx)
		}
	})

	s.queueManager = queue.NewQueueManager(ctx, s.workerPoolManager)
	s.AddCleanupMethod(func(cleanupCtx context.Context) {
		if s.queueManager != nil {
			_ = s.queueManager.Close(cleanupCtx)
		}
	})

	err = s.setupEventsQueue(ctx)
	if err != nil {
		s.AddStartupError(fmt.Errorf("could not setup application events: %w", err))
	}
}

func (s *Service) sendStopError(ctx context.Context, err error) {
	s.errorChannelMutex.Lock()
	defer s.errorChannelMutex.Unlock()

	select {
	case <-ctx.Done():
		return
	default:
	}

	// Preserve first non-nil error if one is already pending.
	if err != nil {
		select {
		case pending := <-s.errorChannel:
			if pending != nil {
				err = pending
			}
		default:
		}
	}

	select {
	case s.errorChannel <- err:
	default:
	}
}
