package tenancy

import (
	"context"

	"connectrpc.com/connect"

	"github.com/pitabwire/frame/v2/security"
)

// NewClaimsInterceptor returns a Connect interceptor that derives
// tenancy.Claims from auth claims and binds them to ctx for downstream
// code. The interceptor performs no database activity — it is cheap
// and safe for streaming RPCs.
//
// Register after the authentication interceptor so auth claims are
// available when this interceptor reads them.
func NewClaimsInterceptor() connect.Interceptor {
	return &claimsInterceptor{}
}

type claimsInterceptor struct{}

func (*claimsInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return next(bindClaims(ctx), req)
	}
}

func (*claimsInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (*claimsInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(bindClaims(ctx), conn)
	}
}

// bindClaims derives Claims from auth claims (if present) and binds
// them. If no auth claims are in ctx, returns ctx unchanged.
func bindClaims(ctx context.Context) context.Context {
	auth := security.ClaimsFromContext(ctx)
	if auth == nil {
		return ctx
	}
	return WithClaims(ctx, ClaimsFromAuth(ctx, auth))
}
