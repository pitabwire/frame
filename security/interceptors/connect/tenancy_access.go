package connect

import (
	"context"

	"connectrpc.com/connect/v2"

	"github.com/pitabwire/frame/v3/security/authorizer"
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

// NewTenancyAccessInterceptor creates a Connect server interceptor that
// enforces tenancy data access using the provided TenancyAccessChecker.
func NewTenancyAccessInterceptor(checker *authorizer.TenancyAccessChecker) connect.ServerInterceptor {
	t := &tenancyAccessInterceptor{checker: checker}
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			if err := t.checkAccess(ctx); err != nil {
				return err
			}
			return next(ctx, spec, stream)
		}
	}
}

func (t *tenancyAccessInterceptor) checkAccess(ctx context.Context) error {
	if err := t.checker.CheckAccess(ctx); err != nil {
		return authorizer.ToConnectError(err)
	}
	return nil
}
