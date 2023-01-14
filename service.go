package frame

import (
	"context"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pitabwire/frame/internal"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/sdk/trace"
	"strings"

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
	name           string
	version        string
	environment    string
	logger         *logrus.Logger
	traceExporter  trace.SpanExporter
	traceSampler   trace.Sampler
	handler        http.Handler
	driver         internal.Server
	grpcServer     *grpc.Server
	listener       net.Listener
	corsPolicy     string
	client         *http.Client
	queue          *queue
	dataStore      *store
	bundle         *i18n.Bundle
	healthCheckers []Checker
	livelinessPath string
	readinessPath  string
	startup        func(s *Service)
	cleanups       []func(ctx context.Context)
	eventRegistry  map[string]EventI
	configuration  interface{}
	once           sync.Once
}

type Option func(service *Service)

// NewService creates a new instance of Service with the name and supplied options
// It is used together with the Init option to setup components of a service that is not yet running.
func NewService(name string, opts ...Option) *Service {
	q := newQueue()

	service := &Service{
		name: name,
		dataStore: &store{
			readDatabase:  []*gorm.DB{},
			writeDatabase: []*gorm.DB{},
		},
		client: &http.Client{},
		queue:  q,
		logger: logrus.New(),
	}

	opts = append(opts, Logger())

	service.Init(opts...)

	return service
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

// Init evaluates the options provided as arguments and supplies them to the service object
func (s *Service) Init(opts ...Option) {
	for _, opt := range opts {
		opt(s)
	}
}

// AddPreStartMethod Adds user defined functions that can be run just before
// the service starts receiving requests but is fully initialized.
func (s *Service) AddPreStartMethod(f func(s *Service)) {
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
	s.cleanups = append(s.cleanups, f)
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
	oauth2Config, ok := s.Config().(ConfigurationOAUTH2)
	if ok {
		oauth2ServiceAdminHost := oauth2Config.GetOauth2ServiceAdminURI()

		clientSecret := oauth2Config.GetOauth2ServiceClientSecret()

		oauth2Audience := oauth2Config.GetOauth2ServiceAudience()

		if oauth2ServiceAdminHost != "" && clientSecret != "" {
			audienceList := strings.Split(oauth2Audience, ",")

			err := s.RegisterForJwtWithParams(ctx, oauth2ServiceAdminHost, s.name, s.name, clientSecret,
				"", audienceList, map[string]string{})
			if err != nil {
				return err
			}
		}
	}

	err := s.initPubsub(ctx)
	if err != nil {
		return err
	}

	if s.handler == nil {
		s.handler = http.DefaultServeMux
	}

	if s.corsPolicy == "" {
		s.corsPolicy = "*"
	}

	return s.initServer(ctx, address)
}

func (s *Service) initServer(ctx context.Context, address string) error {
	err := s.initTracer()
	if err != nil {
		return err
	}

	if s.livelinessPath == "" {
		s.livelinessPath = "/healthz/liveness"
	}

	if s.readinessPath == "" {
		s.readinessPath = "/healthz/readiness"
	}

	s.once.Do(func() {
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
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
					IdleTimeout:  120 * time.Second,
				},
				listener: s.listener,
			}
		}
		if s.driver == nil {
			s.driver = &defaultDriver{
				httpServer: &http.Server{
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
					IdleTimeout:  120 * time.Second,
				},
			}
		}
	})

	if s.startup != nil {
		s.startup(s)
	}

	err = s.driver.ListenAndServe(address, s.handler)
	return err
}

// Stop Used to gracefully run clean up methods ensuring all requests that
// were being handled are completed well without interuptions.
func (s *Service) Stop(ctx context.Context) {
	for {
		var cleanup func(ctx context.Context)
		if len(s.cleanups) > 0 {
			last := len(s.cleanups) - 1
			cleanup = s.cleanups[last]
			s.cleanups = s.cleanups[:last]
		}
		if cleanup == nil {
			return
		}
		cleanup(ctx)
	}
}
