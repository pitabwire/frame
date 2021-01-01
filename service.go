package frame

import (
	"context"
	"errors"
	"gocloud.dev/server"
	"gocloud.dev/server/health"
	"gocloud.dev/server/sdserver"
	"log"
	"net/http"
)

const ctxKeyService = "serviceKey"

type Service struct {
	name   string
	server   *server.Server
	queue  *Queue
	dataStore *store
	healthCheckers []health.Checker
	cleanup func()
}

type Option func(service *Service)

type Options struct {
	ServerOpts *server.Options

}

func Server(opts *server.Options) Option {
	return func(c *Service) {
		c.server = server.New(http.DefaultServeMux, opts)
	}
}

func NewService(name string, opts... Option) *Service {

	defaultSrvOptions := &server.Options{
		RequestLogger: sdserver.NewRequestLogger(),
		Driver:                &server.DefaultDriver{},
	}

	service := &Service{
		name: name,
		server:   server.New(http.DefaultServeMux, defaultSrvOptions),
		dataStore: &store{},
		queue: &Queue{},
	}

	for _, opt := range opts {
		opt(service)
	}

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

func (s *Service) AddCleanupMethod(f func())  {
	if s.cleanup == nil {
		s.cleanup = f
		return
	}

	old := s.cleanup
	s.cleanup = func() { old(); f() }
}

func (s *Service) AddHealthCheck( checker health.Checker ) {
	if s.healthCheckers != nil {
		s.healthCheckers = []health.Checker{}
	}
	s.healthCheckers = append(s.healthCheckers, checker)
}

func (s *Service) Run(ctx context.Context, address string, serverSetup func(context.Context, *server.Server) error ) error {

	if s.server == nil {
		return errors.New("attempting to run service without a server")
	}

	if err := serverSetup(ctx, s.server); err != nil {
		return err
	}

	s.AddCleanupMethod(func() {
		err := s.server.Shutdown(ctx)
		if err != nil {
			log.Printf("Run -- Server could not shut down gracefully : %v", err)
		}
	})

	err := s.initPubsub(ctx)
	if err != nil {
		return err
	}

	err = s.server.ListenAndServe(address)
	return err

}

func (s *Service)Stop() {
	if s.cleanup != nil {
		s.cleanup()
	}
}

