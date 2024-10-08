package frame

import (
	"context"
	"errors"
	"fmt"
	ghandler "github.com/gorilla/handlers"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/panjf2000/ants/v2"
	"github.com/pitabwire/frame/internal"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"os/signal"
	"runtime/debug"
	"syscall"

	"google.golang.org/grpc/health/grpc_health_v1"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"
)

type contextKey string

func (c contextKey) String() string {
	return "frame/" + string(c)
}

const ctxKeyService = contextKey("serviceKey")

// Service framework struct to hold together all application components
// An instance of this type scoped to stay for the lifetime of the application.
// It is pushed and pulled from contexts to make it easy to pass around.
type Service struct {
	name                       string
	jwtClient                  map[string]any
	jwtClientSecret            string
	version                    string
	environment                string
	logger                     *logrus.Logger
	traceExporter              trace.SpanExporter
	traceSampler               trace.Sampler
	handler                    http.Handler
	cancelFunc                 context.CancelFunc
	errorChannelMutex          sync.Mutex
	errorChannel               chan error
	backGroundClient           func(ctx context.Context) error
	poolWorkerCount            int
	poolCapacity               int
	pool                       *ants.MultiPool
	driver                     any
	grpcServer                 *grpc.Server
	grpcServerEnableReflection bool
	priListener                net.Listener
	secListener                net.Listener
	grpcPort                   string
	client                     *http.Client
	queue                      *queue
	dataStore                  *store
	bundle                     *i18n.Bundle
	healthCheckers             []Checker
	healthCheckPath            string
	startup                    func(s *Service)
	cleanup                    func(ctx context.Context)
	eventRegistry              map[string]EventI
	configuration              any
	startOnce                  sync.Once
	stopMutex                  sync.Mutex
}

type Option func(service *Service)

// NewService creates a new instance of Service with the name and supplied options.
// Internally it calls NewServiceWithContext and creates a background context for use.
func NewService(name string, opts ...Option) (context.Context, *Service) {
	ctx := context.Background()
	return NewServiceWithContext(ctx, name, opts...)
}

// NewServiceWithContext creates a new instance of Service with context, name and supplied options
// It is used together with the Init option to setup components of a service that is not yet running.
func NewServiceWithContext(ctx context.Context, name string, opts ...Option) (context.Context, *Service) {

	ctx, cancel := signal.NotifyContext(ctx,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	concurrency := runtime.NumCPU() * 10

	q := newQueue(ctx)

	service := &Service{
		name:            name,
		cancelFunc:      cancel,
		errorChannel:    make(chan error, 1),
		dataStore:       newDataStore(),
		client:          &http.Client{},
		queue:           q,
		poolWorkerCount: concurrency,
		poolCapacity:    100,
	}

	opts = append(opts, Logger())

	service.Init(opts...)

	poolOptions := []ants.Option{
		ants.WithLogger(service.L(ctx)),
		ants.WithNonblocking(true),
	}

	service.pool, _ = ants.NewMultiPool(service.poolWorkerCount, service.poolCapacity, ants.LeastTasks, poolOptions...)

	ctx1 := ToContext(ctx, service)
	ctx1 = ConfigToContext(ctx1, service.Config())
	return ctx1, service
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

// Version gets the release version of the service.
func (s *Service) Version() string {
	return s.version
}

// Environment gets the runtime environment of the service.
func (s *Service) Environment() string {
	return s.environment
}

// JwtClient gets the authenticated jwt client if configured at startup
func (s *Service) JwtClient() map[string]any {
	return s.jwtClient
}

// JwtClientID gets the authenticated jwt client if configured at startup
func (s *Service) JwtClientID() string {
	clientId, ok := s.jwtClient["client_id"].(string)
	if !ok {
		return ""
	}
	return clientId
}

// JwtClientSecret gets the authenticated jwt client if configured at startup
func (s *Service) JwtClientSecret() string {
	return s.jwtClientSecret
}

func (s *Service) H() http.Handler {
	return s.handler
}

// Init evaluates the options provided as arguments and supplies them to the service object
func (s *Service) Init(opts ...Option) {
	for _, opt := range opts {
		opt(s)
	}
}

// AddPreStartMethod Adds user defined functions that can be run just before
// the service starts receiving requests but is fully initialized.
func (s *Service) AddPreStartMethod(f func(s *Service)) {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()
	if s.startup == nil {
		s.startup = f
		return
	}

	old := s.startup
	s.startup = func(st *Service) { old(st); f(st) }
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

// Run is used to actually instantiate the initialised components and
// keep them useful by handling incoming requests
func (s *Service) Run(ctx context.Context, address string) error {

	err := s.initPubsub(ctx)
	if err != nil {
		return err
	}

	//connect the background processor
	if s.backGroundClient != nil {
		go func() {
			err = s.backGroundClient(ctx)
			s.sendStopError(ctx, err)

		}()

	}

	go func() {
		err = s.initServer(ctx, address)
		if err != nil || s.backGroundClient == nil {
			s.sendStopError(ctx, err)
		}

	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err0 := <-s.errorChannel:
		if err0 != nil {
			s.L(ctx).
				WithError(err0).
				WithField("stacktrace", string(debug.Stack())).
				Info("system exit in error")
		} else {
			s.L(ctx).Info("system exit without fuss")
		}
		return err0
	}

}

func (s *Service) initServer(ctx context.Context, httpPort string) error {
	err := s.initTracer(ctx)
	if err != nil {
		return err
	}

	if s.healthCheckPath == "" ||
		s.healthCheckPath == "/" && s.handler != nil {
		s.healthCheckPath = "/healthz"
	}

	if httpPort == "" {
		config, ok := s.Config().(ConfigurationPorts)
		if !ok {
			if s.TLSEnabled() {
				httpPort = ":https"
			} else {
				httpPort = "http"
			}
		} else {
			httpPort = config.HttpPort()
		}

	}

	if s.grpcServer != nil {

		if s.grpcPort == "" {

			config, ok := s.Config().(ConfigurationPorts)
			if !ok {
				s.grpcPort = ":50051"
			}

			s.grpcPort = config.GrpcPort()

		}

		if httpPort == s.grpcPort {
			return fmt.Errorf("HTTP PORT %s and GRPC PORT %s can not be same", httpPort, s.grpcPort)
		}

	}

	s.startOnce.Do(func() {

		mux := http.NewServeMux()

		applicationHandler := s.handler
		if applicationHandler == nil {
			applicationHandler = http.DefaultServeMux
		}

		mux.HandleFunc(s.healthCheckPath, s.HandleHealth)

		mux.Handle("/", applicationHandler)

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

			s.handler = ghandler.CORS(corsOptions...)(mux)
		} else {
			s.handler = mux
		}

		defaultServer := defaultDriver{
			ctx:  ctx,
			log:  s.L(ctx),
			port: httpPort,
			httpServer: &http.Server{
				BaseContext: func(listener net.Listener) context.Context {
					return ctx
				},
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  120 * time.Second,
			},
		}

		// If grpc server is setup we should use the correct driver
		if s.grpcServer != nil {

			if s.grpcPort == "" {

				config, ok := s.Config().(ConfigurationPorts)
				if !ok {
					s.grpcPort = ":50051"
				}

				s.grpcPort = config.GrpcPort()

			}

			grpcHS := NewGrpcHealthServer(s)
			grpc_health_v1.RegisterHealthServer(s.grpcServer, grpcHS)

			if s.grpcServerEnableReflection {
				reflection.Register(s.grpcServer)
			}

			s.driver = &grpcDriver{
				defaultDriver: defaultServer,
				grpcPort:      s.grpcPort,
				grpcServer:    s.grpcServer,
				grpcListener:  s.secListener,
			}
		}

		if s.driver == nil {
			s.driver = &defaultServer
		}
	})

	if s.startup != nil {
		s.startup(s)
	}

	if s.TLSEnabled() {

		config, _ := s.Config().(ConfigurationTLS)

		tlsServer, ok := s.driver.(internal.TLSServer)
		if !ok {
			return errors.New("tls server has to be of type internal.TLSServer")
		}
		return tlsServer.ListenAndServeTLS(httpPort, config.TLSCertPath(), config.TLSCertKeyPath(), s.handler)
	}

	nonTlsServer, ok := s.driver.(internal.Server)
	if !ok {
		return errors.New("server has to be of type internal.Server")
	}
	return nonTlsServer.ListenAndServe(httpPort, s.handler)

}

// Stop Used to gracefully run clean up methods ensuring all requests that
// were being handled are completed well without interuptions.
func (s *Service) Stop(ctx context.Context) {

	if !s.stopMutex.TryLock() {
		return
	}
	defer s.stopMutex.Unlock()

	if s.cleanup != nil {
		s.cleanup(ctx)
	}

	if s.pool != nil {
		s.pool.Free()
	}

	if s.cancelFunc != nil {
		s.cancelFunc()
	}

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
