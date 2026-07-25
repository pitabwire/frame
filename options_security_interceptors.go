package frame

import (
	"context"

	"connectrpc.com/connect"

	"github.com/pitabwire/frame/v2/security"
	connectix "github.com/pitabwire/frame/v2/security/interceptors/connect"
)

// ConnectDefaultInterceptors returns the standard Connect interceptor chain
// wired to this service's tenancy ClaimsBinder.
//
// When Secure Profile / Hybrid is enabled, the binder uses
// HonorInternalSkip=false and RequireClaims=true so missing tenancy fails
// early with FailedPrecondition and internal JWTs bind RLS instead of Skip.
//
// Prefer this over connect.DefaultList when constructing handlers so Secure
// Profile options on the service take effect transparently.
func (s *Service) ConnectDefaultInterceptors(
	ctx context.Context,
	auth security.Authenticator,
	more ...connect.Interceptor,
) ([]connect.Interceptor, error) {
	return connectix.DefaultListWithBinder(ctx, auth, s.ClaimsBinder(), more...)
}
