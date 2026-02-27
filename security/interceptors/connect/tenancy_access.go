package connect

import (
	"context"

	"connectrpc.com/connect"

	"github.com/pitabwire/frame/security/authorizer"
)

// tenancyAccessInterceptor is a Connect interceptor that verifies the caller
// has data access to the partition identified in their claims. It uses
// TenancyAccessChecker.CheckAccess which checks the "member" relation for
// regular users and the "service" relation for system_internal callers.
//
// This interceptor should be placed after the authentication interceptor
// in the interceptor chain so that claims are available in the context.
type tenancyAccessInterceptor struct {
	checker *authorizer.TenancyAccessChecker
}

// NewTenancyAccessInterceptor creates a Connect interceptor that enforces
// tenancy data access using the provided TenancyAccessChecker.
func NewTenancyAccessInterceptor(checker *authorizer.TenancyAccessChecker) connect.Interceptor {
	return &tenancyAccessInterceptor{checker: checker}
}

func (t *tenancyAccessInterceptor) checkAccess(ctx context.Context) error {
	if err := t.checker.CheckAccess(ctx); err != nil {
		return authorizer.ToConnectError(err)
	}
	return nil
}

// WrapUnary implements the unary interceptor for tenancy access checking.
func (t *tenancyAccessInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := t.checkAccess(ctx); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient implements the streaming client interceptor (pass-through).
func (t *tenancyAccessInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler implements the streaming handler interceptor for tenancy access.
func (t *tenancyAccessInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if err := t.checkAccess(ctx); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}
