package frame

import (
	"context"
	"fmt"
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

const ctxKeyService = "serviceKey"

type Service struct {
	name           string
	server         *server.Server
	handler        http.Handler
	serverOptions  *server.Options
	grpcServer     *grpc.Server
	listener       net.Listener
	client         *http.Client
	queue          *Queue
	dataStore      *store
	healthCheckers []health.Checker
	cleanup        func()
}

type Option func(service *Service)

func NewService(name string, opts ...Option) *Service {

	service := &Service{
		name:      name,
		dataStore: &store{
			readDatabase: []*gorm.DB{},
			writeDatabase: []*gorm.DB{},
		},
		client: &http.Client{},
		queue:     &Queue{},
	}

	service.Init(opts...)

	return service
}

func ToContext(ctx context.Context, service *Service) context.Context {
	return context.WithValue(ctx, ctxKeyService, service)
}

func FromContext(ctx context.Context) *Service {
	service, ok := ctx.Value(ctxKeyService).(*Service)
	if !ok {
		return nil
	}

	return service
}

func (s *Service) Init(opts ...Option) {

	for _, opt := range opts {
		opt(s)
	}
}

func (s *Service) AddCleanupMethod(f func()) {
	if s.cleanup == nil {
		s.cleanup = f
		return
	}

	old := s.cleanup
	s.cleanup = func() { old(); f() }
}

func (s *Service) AddHealthCheck(checker health.Checker) {
	if s.healthCheckers != nil {
		s.healthCheckers = []health.Checker{}
	}
	s.healthCheckers = append(s.healthCheckers, checker)
}

func (s *Service) Run(ctx context.Context, address string) error {

	err := s.initPubsub(ctx)
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
		s.serverOptions.RequestLogger = requestlog.NewNCSALogger(os.Stdout, func(e error) { fmt.Println(e) })
	}


	// If grpc server is setup we should use the correct driver
	if s.grpcServer != nil {

		s.serverOptions.Driver = &grpcDriver{
			grpcServer: s.grpcServer,
			httpServer: &http.Server{
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
				IdleTimeout:  120 * time.Second,
			},
			listener: s.listener,
		}

	}

	s.server = server.New(s.handler, s.serverOptions)

	err = s.server.ListenAndServe(address)
	return err
}

func (s *Service) Stop() {
	if s.cleanup != nil {
		s.cleanup()
	}
}
