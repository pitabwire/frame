package tenancy

import (
	"context"
	"net/http"

	"connectrpc.com/connect/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewClaimsInterceptor returns a Connect server interceptor that derives
// tenancy.Claims from auth claims and binds them to ctx.
// Zero-arg form uses a legacy binder (HonorInternalSkip=true, RequireClaims=false).
//
// Register after the authentication interceptor so auth claims are available.
func NewClaimsInterceptor() connect.ServerInterceptor {
	return NewClaimsInterceptorWithBinder(nil)
}

// NewClaimsInterceptorWithBinder uses the supplied binder. Nil uses legacy defaults.
// A binding failure fails the RPC with connect.CodeFailedPrecondition.
func NewClaimsInterceptorWithBinder(b *ClaimsBinder) connect.ServerInterceptor {
	if b == nil {
		b = NewClaimsBinder(false)
	}
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			boundCtx, err := b.Bind(ctx)
			if err != nil {
				return connect.NewError(connect.CodeFailedPrecondition, err.Error()).WithCause(err)
			}
			return next(boundCtx, spec, stream)
		}
	}
}

// UnaryClaimsInterceptor returns a gRPC unary interceptor that binds tenancy claims.
func UnaryClaimsInterceptor(b *ClaimsBinder) grpc.UnaryServerInterceptor {
	if b == nil {
		b = NewClaimsBinder(false)
	}
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		ctx, err := b.Bind(ctx)
		if err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		return handler(ctx, req)
	}
}

// StreamClaimsInterceptor returns a gRPC stream interceptor that binds tenancy claims.
func StreamClaimsInterceptor(b *ClaimsBinder) grpc.StreamServerInterceptor {
	if b == nil {
		b = NewClaimsBinder(false)
	}
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx, err := b.Bind(ss.Context())
		if err != nil {
			return status.Error(codes.FailedPrecondition, err.Error())
		}
		return handler(srv, &claimsServerStream{ServerStream: ss, ctx: ctx})
	}
}

type claimsServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *claimsServerStream) Context() context.Context { return s.ctx }

// ClaimsMiddleware returns HTTP middleware that binds tenancy claims.
func ClaimsMiddleware(next http.Handler, b *ClaimsBinder) http.Handler {
	if b == nil {
		b = NewClaimsBinder(false)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := b.Bind(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
