package manager

import (
	"context"

	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
	"github.com/pitabwire/frame/security/openid"
)

// SecurityConfiguration combines all configuration interfaces needed by the security manager.
type SecurityConfiguration interface {
	config.ConfigurationOAUTH2
	config.ConfigurationJWTVerification
	config.ConfigurationAuthorization
}

// managerImpl is the concrete implementation of the Manager interface.
type managerImpl struct {
	clientID        string
	clientSecret    string
	jwtClient       map[string]any
	clientRegistrar security.Oauth2ClientRegistrar
	authenticator   security.Authenticator
	authorizer      security.Authorizer
}

// NewManager creates and returns a new security Manager.
func NewManager(_ context.Context, serviceName, serviceEnv string,
	cfg SecurityConfiguration, invoker client.Manager) security.Manager {
	return &managerImpl{
		clientRegistrar: openid.NewClientRegistrar(serviceName, serviceEnv, cfg, invoker),
		authenticator:   openid.NewJwtTokenAuthenticator(cfg),
		authorizer: authorizer.NewKetoAdapter(
			cfg,
			invoker,
			authorizer.NewAuditLogger(authorizer.AuditLoggerConfig{}),
		),
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
