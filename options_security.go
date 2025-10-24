package frame

import (
	"context"

	"github.com/pitabwire/frame/security"
)

func WithRegisterServerOauth2Client() Option {
	return func(ctx context.Context, svc *Service) {
		sm := svc.SecurityManager()
		clr := sm.GetOauth2ClientRegistrar(ctx)

		// Register for JWT
		err := clr.RegisterForJwt(ctx, sm)
		if err != nil {
			svc.Log(ctx).WithError(err).Fatal("main -- could not register for jwt")
		}
	}
}

func (s *Service) SecurityManager() security.Manager {
	return s.securityManager
}
