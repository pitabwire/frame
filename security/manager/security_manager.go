package manager

import (
	"context"

	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/openid"
	"github.com/pitabwire/frame/security/permissions"
)

// managerImpl is the concrete implementation of the Manager interface.
type managerImpl struct {
	jwtClient          map[string]any
	serviceName        string
	serviceEnvironment string
	cfg                config.ConfigurationOAUTH2
	clientRegistrar    security.Oauth2ClientRegistrar
	authenticator      security.Authenticator
	authorizer         security.Authorizer
}

// NewManager creates and returns a new security Manager.
func NewManager(_ context.Context,
	cfg *config.ConfigurationDefault,
	invoker client.Manager) security.Manager {
	return &managerImpl{
		serviceName:        cfg.Name(),
		serviceEnvironment: cfg.Environment(),
		cfg:                cfg,
		clientRegistrar:    openid.NewClientRegistrar(cfg.Name(), cfg.Environment(), cfg),
		authenticator:      openid.NewJwtTokenAuthenticator(cfg),
		authorizer:         permissions.NewKetoAuthorizer(cfg, invoker),
	}
}

func (s *managerImpl) GetOauth2ClientRegistrar(_ context.Context) security.Oauth2ClientRegistrar {
	return s.clientRegistrar
}

func (s *managerImpl) GetAuthenticator(_ context.Context) security.Authenticator {
	return s.authenticator
}

func (s *managerImpl) GetAuthorizer(_ context.Context) security.Authorizer {
	return s.authorizer
}
