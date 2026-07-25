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
// Uses a legacy claims binder (HonorInternalSkip=true). For Secure Profile
// use DefaultListWithBinder.
func DefaultList(
	ctx context.Context,
	authI security.Authenticator,
	moreInterceptors ...connect.Interceptor,
) ([]connect.Interceptor, error) {
	return DefaultListWithBinder(ctx, authI, nil, moreInterceptors...)
}

// DefaultListWithBinder is DefaultList with an explicit tenancy.ClaimsBinder.
// Nil binder keeps legacy behaviour. Secure Profile passes a binder with
// HonorInternalSkip=false and RequireClaims=true.
func DefaultListWithBinder(
	_ context.Context,
	authI security.Authenticator,
	binder *tenancy.ClaimsBinder,
	moreInterceptors ...connect.Interceptor,
) ([]connect.Interceptor, error) {
	var interceptorList []connect.Interceptor

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, err
	}

	claimsIx := tenancy.NewClaimsInterceptor()
	if binder != nil {
		claimsIx = tenancy.NewClaimsInterceptorWithBinder(binder)
	}

	interceptorList = append(
		interceptorList,
		otelInterceptor,
		NewValidationInterceptor(),
		NewAuthInterceptor(authI),
		claimsIx,
	)
	interceptorList = append(interceptorList, moreInterceptors...)

	return interceptorList, nil
}
