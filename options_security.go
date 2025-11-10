package frame

import (
	"context"

	"github.com/pitabwire/frame/security"
)

func WithRegisterServerOauth2Client() Option {
	return func(_ context.Context, svc *Service) {
		svc.registerOauth2Cli = true
	}
}

func (s *Service) SecurityManager() security.Manager {
	return s.securityManager
}
