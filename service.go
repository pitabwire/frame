package frame

import (
	"context"
	"fmt"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"gocloud.dev/server"
	"gocloud.dev/server/health"
	"gocloud.dev/server/requestlog"
	"google.golang.org/grpc"
	"gorm.io/gorm"
	"net"
	"net/http"
	"os"
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
	logger         ILogger
	server         *server.Server
	handler        http.Handler
	serverOptions  *server.Options
	grpcServer     *grpc.Server
	listener       net.Listener
	corsPolicy     string
	client         *http.Client
	queue          *queue
	queuePath      string
	dataStore      *store
	bundle         *i18n.Bundle
	healthCheckers []health.Checker
	startup        func(s *Service)
	cleanup        func()
	eventRegistry  map[string]EventI
}

type Option func(service *Service)

// NewService creates a new instance of Service with the name and supplied options
// It is used together with the Init option to setup components of a service that is not yet running.
func NewService(name string, opts ...Option) *Service {
	q, _ := newQueue()

	service := &Service{
		name: name,
		dataStore: &store{
			readDatabase:  []*gorm.DB{},
			writeDatabase: []*gorm.DB{},
		},
		client: &http.Client{},
		queue:  q,
	}

	opts = append(opts, Logger())

	service.Init(opts...)

	return service
}

// ToContext pushes a service instance into the supplied context for easier propagation
func ToContext(ctx context.Context, service *Service) context.Context {
	return context.WithValue(ctx, ctxKeyService, service)
}

// FromContext obtains a service instance being propagated through the context
func FromContext(ctx context.Context) *Service {
	service, ok := ctx.Value(ctxKeyService).(*Service)
	if !ok {
		return nil
	}

	return service
}

// Name gets the name of the service. Its the first argument used when NewService is called
func (s *Service) Name() string {
	return s.name
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
func (s *Service) AddCleanupMethod(f func()) {
	if s.cleanup == nil {
		s.cleanup = f
		return
	}

	old := s.cleanup
	s.cleanup = func() { old(); f() }
}

// AddHealthCheck Adds health checks that are run periodically to ascertain the system is ok
// The arguments are implementations of the checker interface and should work with just about
// any system that is given to them.
func (s *Service) AddHealthCheck(checker health.Checker) {
	if s.healthCheckers != nil {
		s.healthCheckers = []health.Checker{}
	}
	s.healthCheckers = append(s.healthCheckers, checker)
}

// Run is used to actually instantiate the initialised components and
// keep them useful by handling incoming requests
func (s *Service) Run(ctx context.Context, address string) error {

	err := s.registerForJwt(ctx)
	if err != nil {
		return err
	}

	// Whenever the registry is not empty the events queue is automatically initiated
	if s.eventRegistry != nil && len(s.eventRegistry) > 0 {
		eventsQueueHandler := eventQueueHandler{
			service: s,
		}
		eventsQueueURL := GetEnv(envEventsQueueUrl, fmt.Sprintf("mem://%s", eventsQueueName))
		eventsQueue := RegisterSubscriber(eventsQueueName, eventsQueueURL, 10, &eventsQueueHandler)
		eventsQueue(s)
		eventsQueueP := RegisterPublisher(eventsQueueName, eventsQueueURL)
		eventsQueueP(s)
	}

	err = s.initPubsub(ctx)
	if err != nil {
		return err
	}

	if s.handler == nil {
		s.handler = http.DefaultServeMux
	}

	if s.serverOptions == nil {
		s.serverOptions = &server.Options{}
	}

	if s.serverOptions.RequestLogger == nil {
		s.serverOptions.RequestLogger = requestlog.NewNCSALogger(
			os.Stdout,
			func(e error) { s.logger.Error(e.Error()) })
	}

	if s.corsPolicy == "" {
		s.corsPolicy = "*"
	}

	if s.queuePath == "" {
		s.queuePath = "/queue"
	}

	s.queuePath = fmt.Sprintf("%s/{queueReferencePath}", s.queuePath)

	p, err := cloudevents.NewHTTP()
	if err != nil {
		return err
	}

	queueHandler, err := cloudevents.NewHTTPReceiveHandler(ctx, p, receiveCloudEvents)
	if err != nil {
		return err
	}

	h := s.handler

	mux := http.NewServeMux()
	mux.Handle(s.queuePath, queueHandler)
	mux.Handle("/", h)

	s.handler = mux

	// If grpc server is setup we should use the correct driver
	if s.grpcServer != nil {
		s.serverOptions.Driver = &grpcDriver{
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

	s.server = server.New(s.handler, s.serverOptions)

	if s.startup != nil {
		s.startup(s)
	}

	err = s.server.ListenAndServe(address)
	return err
}

// Stop Used to gracefully run clean up methods ensuring all requests that
// were being handled are completed well without interuptions.
func (s *Service) Stop() {
	if s.cleanup != nil {
		s.cleanup()
	}
}
