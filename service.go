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

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/localization"
	"github.com/pitabwire/frame/security"
	securityManager "github.com/pitabwire/frame/security/manager"
	"github.com/pitabwire/util"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	sdklogs "go.opentelemetry.io/otel/sdk/log"
	sdkmetrics "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"gocloud.dev/server/driver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
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
	name           string
	jwtClient      map[string]any
	version        string
	environment    string
	logger         *util.LogEntry
	disableTracing bool

	traceTextMap      propagation.TextMapPropagator
	traceExporter     sdktrace.SpanExporter
	traceSampler      sdktrace.Sampler
	metricsReader     sdkmetrics.Reader
	traceLogsExporter sdklogs.Exporter
	handler           http.Handler
	cancelFunc        context.CancelFunc
	errorChannelMutex sync.Mutex
	errorChannel      chan error

	backGroundClient           func(ctx context.Context) error
	pool                       WorkerPool
	poolOptions                *WorkerPoolOptions
	driver                     ServerDriver
	grpcServer                 *grpc.Server
	grpcServerEnableReflection bool
	grpcListener               net.Listener
	grpcPort                   string
	client                     *http.Client
	queue                      *queue
	dataStores                 sync.Map
	bundle                     *i18n.Bundle
	healthCheckers             []Checker
	healthCheckPath            string
	startup                    func(ctx context.Context, s *Service)
	cleanup                    func(ctx context.Context)
	eventRegistry              map[string]EventI

	configuration any

	localizationManager localization.Manager
	securityManager     security.Manager
	cacheManager        *cache.Manager

	startOnce sync.Once
	stopMutex sync.Mutex
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
	defaultLogger := util.Log(ctx)
	ctx = util.ContextWithLogger(ctx, defaultLogger)

	cfg, _ := config.FromEnv[config.ConfigurationDefault]()

	defaultPoolOpts := defaultWorkerPoolOpts(&cfg, defaultLogger)
	defaultPool, err := setupWorkerPool(ctx, defaultPoolOpts)
	if err != nil {
		defaultLogger.WithError(err).Panic("could not create a default worker pool")
	}

	q := newQueue(ctx)

	defaultClient := client.NewHTTPClient(
		client.WithHTTPTimeout(time.Duration(defaultHTTPTimeoutSeconds)*time.Second),
		client.WithHTTPIdleTimeout(time.Duration(defaultHTTPIdleTimeoutSeconds)*time.Second),
	)

	invoker := client.NewInvoker(&cfg, defaultClient)

	svc := &Service{
		name:         name,
		cancelFunc:   signalCancelFunc, // Store its cancel function
		errorChannel: make(chan error, 1),
		client:       defaultClient,
		logger:       defaultLogger,

		pool:        defaultPool,
		poolOptions: defaultPoolOpts,

		configuration: &cfg,
		queue:         q,
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

	if cfg.OpenTelemetryDisable {
		opts = append(opts, WithDisableTracing())
	}

	opts = append(opts, WithLogger()) // Ensure logger is initialized early

	svc.Init(ctx, opts...) // Apply all options, using the signal-aware context

	// Create security manager AFTER options are applied so it gets the correct config
	finalCfg, ok := svc.Config().(*config.ConfigurationDefault)
	if !ok {
		finalCfg = &cfg
	}
	svc.securityManager = securityManager.NewManager(ctx, finalCfg, invoker)

	err = svc.initTracer(ctx)
	if err != nil {
		svc.logger.WithError(err).Panic("could not setup application telemetry")
	}

	// Prepare context to be returned, embedding svc and config
	ctx = ToContext(ctx, svc)
	ctx = config.ToContext(ctx, svc.Config())
	ctx = util.ContextWithLogger(ctx, svc.logger)
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

// WithHTTPClient configures the HTTP client used by the service.
// This allows customizing the HTTP client's behavior such as timeout, transport, etc.
func WithHTTPClient(opts ...client.HTTPOption) Option {
	return func(_ context.Context, s *Service) {
		s.client = client.NewHTTPClient(opts...)
	}
}

func (s *Service) H() http.Handler {
	return s.handler
}

func (s *Service) Security() security.Manager {
	return s.securityManager
}

// Init evaluates the options provided as arguments and supplies them to the service object.
func (s *Service) Init(ctx context.Context, opts ...Option) {
	for _, opt := range opts {
		opt(ctx, s)
	}
}

// AddPreStartMethod Adds user defined functions that can be run just before
// the service starts receiving requests but is fully initialized.
func (s *Service) AddPreStartMethod(f func(ctx context.Context, s *Service)) {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()
	if s.startup == nil {
		s.startup = f
		return
	}

	old := s.startup
	s.startup = func(ctx context.Context, st *Service) { old(ctx, st); f(ctx, st) }
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
	pubSubErr := s.initPubsub(ctx)
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
		if s.disableTracing {
			s.handler = rootHandler
		} else {
			s.handler = otelhttp.NewHandler(rootHandler, s.Name())
		}
		s.initializeServerDrivers(ctx, httpPort)
	})

	if s.startup != nil {
		s.startup(ctx, s)
	}

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

	// Release the worker pool.
	if s.pool != nil {
		s.logger.Info("shutting down worker pool")
		s.pool.Shutdown()
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
