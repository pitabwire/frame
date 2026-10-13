package frame

import (
	"context"

	"connectrpc.com/connect/v2"

	"github.com/pitabwire/frame/v3/security"
	connectix "github.com/pitabwire/frame/v3/security/interceptors/connect"
)

// ConnectDefaultInterceptors returns the standard Connect interceptor chain
// wired to this service's tenancy ClaimsBinder.
//
// When Secure Profile / Hybrid is enabled, the binder uses
// HonorInternalSkip=false and RequireClaims=true so missing tenancy fails
// early with FailedPrecondition and internal JWTs bind RLS instead of Skip.
//
// Pass the result to connect.NewServer, then register handlers on that server
// and mount it with connecthttp.Mount. Prefer this over connectix.DefaultList so
// Secure Profile options on the service take effect transparently.
func (s *Service) ConnectDefaultInterceptors(
	ctx context.Context,
	auth security.Authenticator,
	more ...connect.ServerInterceptor,
) ([]connect.ServerInterceptor, error) {
	return connectix.DefaultListWithBinder(ctx, auth, s.ClaimsBinder(), more...)
}
