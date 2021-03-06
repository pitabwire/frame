package frame

import (
	"context"
	"errors"
	"fmt"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"gocloud.dev/server"
	"gocloud.dev/server/health"
	"gocloud.dev/server/requestlog"
	"google.golang.org/grpc"
	"gorm.io/gorm"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const ctxKeyService = "serviceKey"

const envOauth2ServiceClientSecret = "OAUTH2_SERVICE_CLIENT_SECRET"
const envOauth2ServiceAdminUri = "OAUTH2_SERVICE_ADMIN_URI"
const envOauth2Audience = "OAUTH2_SERVICE_AUDIENCE"

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
	bundle         *i18n.Bundle
	healthCheckers []health.Checker
	startup        func(s *Service)
	cleanup        func()
}

type Option func(service *Service)

func NewService(name string, opts ...Option) *Service {

	service := &Service{
		name: name,
		dataStore: &store{
			readDatabase:  []*gorm.DB{},
			writeDatabase: []*gorm.DB{},
		},
		client: &http.Client{},
		queue:  &Queue{},
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

func (s *Service) registerForJwt(ctx context.Context) error {

	host := GetEnv(envOauth2ServiceAdminUri, "")
	if host == "" {
		return nil
	}
	clientSecret := GetEnv(envOauth2ServiceClientSecret, "")
	if clientSecret == "" {
		return nil
	}

	audienceList := strings.Split(GetEnv(envOauth2Audience, ""), ",")

	oauth2AdminUri := fmt.Sprintf("%s%s", host, "/clients")
	oauth2AdminIDUri := fmt.Sprintf("%s/%s", oauth2AdminUri, s.name)

	status, result, err := s.InvokeRestService(ctx, http.MethodGet, oauth2AdminIDUri, make(map[string]interface{}), nil)
	if err != nil {
		return err
	}

	if status == 200 {
		return nil
	}

	payload := map[string]interface{}{
		"client_id":     s.name,
		"client_name":   s.name,
		"client_secret": clientSecret,
		"grant_types":   []string{"client_credentials"},
		"metadata":      map[string]string{"cc_bot": "true"},
		"aud":           audienceList,
	}

	status, result, err = s.InvokeRestService(ctx, http.MethodPost, oauth2AdminUri, payload, nil)
	if err != nil {
		return err
	}

	if status > 299 || status < 200 {
		return errors.New(fmt.Sprintf(" invalid response status %d had message %s", status, string(result)))
	}
	return nil
}

func (s *Service) Init(opts ...Option) {

	for _, opt := range opts {
		opt(s)
	}
}

func (s *Service) AddPreStartMethod(f func(s *Service)) {
	if s.startup == nil {
		s.startup = f
		return
	}

	old := s.startup
	s.startup = func(st *Service) { old(st); f(st) }
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

	err := s.registerForJwt(ctx)
	if err != nil {
		return err
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

	if s.startup != nil {
		s.startup(s)
	}

	err = s.server.ListenAndServe(address)
	return err
}

func (s *Service) Stop() {
	if s.cleanup != nil {
		s.cleanup()
	}
}
