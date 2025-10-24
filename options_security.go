package frame

import (
	"context"
)

func WithRegisterServerOauth2Client() Option {
	return func(ctx context.Context, svc *Service) {
		sm := svc.Security()
		clr := sm.GetOauth2ClientRegistrar(ctx)

		// Register for JWT
		err := clr.RegisterForJwt(ctx, sm)
		if err != nil {
			svc.Log(ctx).WithError(err).Fatal("main -- could not register for jwt")
		}
	}
}
