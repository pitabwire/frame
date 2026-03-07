package manager

import (
	"context"

	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
	"github.com/pitabwire/frame/security/openid"
	"github.com/pitabwire/frame/security/workloadapi"
)

// SecurityConfiguration combines all configuration interfaces needed by the security manager.
type SecurityConfiguration interface {
	config.ConfigurationWorkloadAPI
	config.ConfigurationOAUTH2
	config.ConfigurationJWTVerification
	config.ConfigurationAuthorization
}

// managerImpl is the concrete implementation of the Manager interface.
type managerImpl struct {
	workloadAPI   security.WorkloadAPI
	authenticator security.Authenticator
	authorizer    security.Authorizer
}

// NewManager creates and returns a new security Manager.
func NewManager(_ context.Context, _, _ string,
	cfg SecurityConfiguration, _ client.Manager) security.Manager {
	return &managerImpl{
		workloadAPI:   workloadapi.NewWorkloadAPI(cfg),
		authenticator: openid.NewJwtTokenAuthenticator(cfg),
		authorizer: authorizer.NewKetoAdapter(
			cfg,
			authorizer.NewAuditLogger(authorizer.AuditLoggerConfig{}),
		),
	}
}

func (s *managerImpl) GetWorkloadAPI(_ context.Context) security.WorkloadAPI {
	return s.workloadAPI
}

func (s *managerImpl) GetAuthenticator(_ context.Context) security.Authenticator {
	return s.authenticator
}

func (s *managerImpl) GetAuthorizer(_ context.Context) security.Authorizer {
	return s.authorizer
}

// Close releases resources held by the security manager components.
func (s *managerImpl) Close() {
	type closer interface {
		Close()
	}

	if c, ok := s.authenticator.(closer); ok {
		c.Close()
	}

	if c, ok := s.authorizer.(closer); ok {
		c.Close()
	}

	if c, ok := s.workloadAPI.(closer); ok {
		c.Close()
	}
}
