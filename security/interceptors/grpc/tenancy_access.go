package grpc

import (
	"context"

	"google.golang.org/grpc"

	"github.com/pitabwire/frame/security/authorizer"
)

// UnaryTenancyAccessInterceptor is a gRPC unary server interceptor that
// verifies the caller has data access to the partition identified in their
// claims. It uses TenancyAccessChecker.CheckAccess which checks the "member"
// relation for regular users and the "service" relation for system_internal
// callers.
//
// This interceptor should be placed after UnaryAuthInterceptor in the
// interceptor chain so that claims are available in the context.
func UnaryTenancyAccessInterceptor(checker *authorizer.TenancyAccessChecker) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := checker.CheckAccess(ctx); err != nil {
			return nil, authorizer.ToGrpcError(err)
		}
		return handler(ctx, req)
	}
}

// StreamTenancyAccessInterceptor is a gRPC stream server interceptor that
// verifies the caller has data access to the partition identified in their
// claims.
//
// This interceptor should be placed after StreamAuthInterceptor in the
// interceptor chain so that claims are available in the context.
func StreamTenancyAccessInterceptor(checker *authorizer.TenancyAccessChecker) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := checker.CheckAccess(ss.Context()); err != nil {
			return authorizer.ToGrpcError(err)
		}
		return handler(srv, ss)
	}
}
