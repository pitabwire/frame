package connect

import (
	"context"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	"github.com/pitabwire/frame/v2/security"
	"github.com/pitabwire/frame/v2/tenancy"
)

// DefaultList returns the standard chain of Connect interceptors used
// by frame services. Order matters: otel → validation → auth → tenancy
// claims. The tenancy interceptor binds tenancy.Claims derived from the
// authenticated principal so downstream pool.DB(ctx, _) queries are
// transparently RLS-scoped without any additional wiring.
//
// Caller-supplied moreInterceptors are appended after this chain.
func DefaultList(
	_ context.Context,
	authI security.Authenticator,
	moreInterceptors ...connect.Interceptor,
) ([]connect.Interceptor, error) {
	var interceptorList []connect.Interceptor

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, err
	}

	interceptorList = append(
		interceptorList,
		otelInterceptor,
		NewValidationInterceptor(),
		NewAuthInterceptor(authI),
		tenancy.NewClaimsInterceptor(),
	)
	interceptorList = append(interceptorList, moreInterceptors...)

	return interceptorList, nil
}
