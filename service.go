package frame

import (
	"context"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pitabwire/frame/internal"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/sdk/trace"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"google.golang.org/grpc"
	"gorm.io/gorm"
	"net"
	"net/http"
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
	name             string
	JwtClientId      string
	version          string
	environment      string
	logger           *logrus.Logger
	logLevel         *logrus.Level
	traceExporter    trace.SpanExporter
	traceSampler     trace.Sampler
	handler          http.Handler
	cancelFunc       context.CancelFunc
	errorChannel     chan error
	backGroundClient func(ctx context.Context) error
	workerCount      int
	pool             *pool
	driver           internal.Server
	grpcServer       *grpc.Server
	listener         net.Listener
	corsPolicy       string
	client           *http.Client
	queue            *queue
	dataStore        *store
	bundle           *i18n.Bundle
	healthCheckers   []Checker
	livelinessPath   string
	readinessPath    string
	startup          func(s *Service)
	cleanup          func(ctx context.Context)
	eventRegistry    map[string]EventI
	configuration    interface{}
	startOnce        sync.Once
	closeOnce        sync.Once
	mu               sync.Mutex
}

type Option func(service *Service)

// NewService creates a new instance of Service with the name and supplied options
// It is used together with the Init option to setup components of a service that is not yet running.
func NewService(name string, opts ...Option) (context.Context, *Service) {

	ctx0, cancel := context.WithCancel(context.Background())

	concurrency := runtime.NumCPU() + 1

	q := newQueue(ctx0)

	service := &Service{
		name:         name,
		cancelFunc:   cancel,
		errorChannel: make(chan error, 1),
		dataStore: &store{
			readDatabase:  []*gorm.DB{},
			writeDatabase: []*gorm.DB{},
		},
		client:      &http.Client{},
		queue:       q,
		logger:      logrus.New(),
		workerCount: concurrency,
	}

	service.pool = newPool(service.L(), concurrency)

	signal.NotifyContext(ctx0,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	opts = append(opts, Logger())

	service.Init(opts...)
	ctx1 := ToContext(ctx0, service)
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

// LogLevel gets the loglevel of the service as set at startup
func (s *Service) LogLevel() *logrus.Level {
	return s.logLevel
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
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()

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

	logger := s.L()

	oauth2Config, ok := s.Config().(ConfigurationOAUTH2)
	if ok {
		oauth2ServiceAdminHost := oauth2Config.GetOauth2ServiceAdminURI()

		clientSecret := oauth2Config.GetOauth2ServiceClientSecret()

		oauth2Audience := oauth2Config.GetOauth2ServiceAudience()

		if oauth2ServiceAdminHost != "" && clientSecret != "" {
			audienceList := strings.Split(oauth2Audience, ",")

			clientID, err := s.RegisterForJwtWithParams(ctx, oauth2ServiceAdminHost, s.Name(), clientSecret,
				"", audienceList, map[string]string{})
			if err != nil {
				return err
			}

			s.JwtClientId = clientID
		}
	}

	err := s.initPubsub(ctx)
	if err != nil {
		return err
	}

	//connect the background processor
	if s.backGroundClient != nil {
		go func() {
			err = s.backGroundClient(ctx)
			s.errorChannel <- err

		}()

	}

	if s.pool != nil {
		s.pool.Start(ctx)
	}
	// connect the server handlers
	if s.handler == nil {
		s.handler = http.DefaultServeMux
	}

	if s.corsPolicy == "" {
		s.corsPolicy = "*"
	}

	go func() {
		err = s.initServer(ctx, address)
		if err != nil {
			s.errorChannel <- err
		} else {
			if s.backGroundClient == nil {
				select {
				case <-s.errorChannel:
					// channel is already closed hence avoid
					return
				default:
					s.errorChannel <- nil
				}

			}

		}

	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err0 := <-s.errorChannel:
		if err0 != nil {
			logger.WithError(err0).Info("system exit in error")
		} else {
			logger.Info("system exit without fuss")
		}
		return err0
	}

}

func (s *Service) initServer(ctx context.Context, address string) error {
	err := s.initTracer(ctx)
	if err != nil {
		return err
	}

	if s.livelinessPath == "" {
		s.livelinessPath = "/healthz/liveness"
	}

	if s.readinessPath == "" {
		s.readinessPath = "/healthz/readiness"
	}

	s.startOnce.Do(func() {
		mux := http.NewServeMux()
		mux.HandleFunc(s.livelinessPath, HandleLive)
		mux.Handle(s.readinessPath, s)
		mux.Handle("/", s.handler)

		s.handler = mux

		// If grpc server is setup we should use the correct driver
		if s.grpcServer != nil {
			s.driver = &grpcDriver{

				corsPolicy: s.corsPolicy,
				grpcServer: s.grpcServer,
				wrappedGrpcServer: grpcweb.WrapServer(
					s.grpcServer,
					grpcweb.WithOriginFunc(func(origin string) bool { return true }),
				),
				httpServer: &http.Server{
					BaseContext: func(listener net.Listener) context.Context {
						return ctx
					},
					ReadTimeout:  5 * time.Second,
					WriteTimeout: 10 * time.Second,
					IdleTimeout:  120 * time.Second,
				},
				listener: s.listener,
			}
		}
		if s.driver == nil {
			s.driver = &defaultDriver{
				httpServer: &http.Server{
					BaseContext: func(listener net.Listener) context.Context {
						return ctx
					},
					ReadTimeout:  5 * time.Second,
					WriteTimeout: 10 * time.Second,
					IdleTimeout:  120 * time.Second,
				},
			}
		}
	})

	if s.startup != nil {
		s.startup(s)
	}

	return s.driver.ListenAndServe(address, s.handler)
}

// Stop Used to gracefully run clean up methods ensuring all requests that
// were being handled are completed well without interuptions.
func (s *Service) Stop(ctx context.Context) {

	s.mu.Lock()
	defer s.mu.Unlock()

	s.closeOnce.Do(func() {
		if s.cleanup != nil {
			s.cleanup(ctx)
		}

		if s.pool != nil {
			err := s.pool.Close()
			if err != nil {
				s.L().WithError(err).Error("could not stop pool")
			}
		}

		if s.cancelFunc != nil {
			s.cancelFunc()
		}

		close(s.errorChannel)
	})
}
