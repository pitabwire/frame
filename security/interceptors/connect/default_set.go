package connect

import (
	"context"

	"connectrpc.com/connect/v2"
	"connectrpc.com/otelconnect"

	"github.com/pitabwire/frame/v3/security"
	"github.com/pitabwire/frame/v3/tenancy"
)

// DefaultList returns the standard chain of Connect server interceptors used
// by frame services. Pass the result to connect.NewServer. Order matters: otel → validation → auth → tenancy
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
	moreInterceptors ...connect.ServerInterceptor,
) ([]connect.ServerInterceptor, error) {
	return DefaultListWithBinder(ctx, authI, nil, moreInterceptors...)
}

// DefaultListWithBinder is DefaultList with an explicit tenancy.ClaimsBinder.
// Nil binder keeps legacy behaviour. Secure Profile passes a binder with
// HonorInternalSkip=false and RequireClaims=true.
func DefaultListWithBinder(
	_ context.Context,
	authI security.Authenticator,
	binder *tenancy.ClaimsBinder,
	moreInterceptors ...connect.ServerInterceptor,
) ([]connect.ServerInterceptor, error) {
	var interceptorList []connect.ServerInterceptor

	otelInterceptor, err := otelconnect.NewServerInterceptor()
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
