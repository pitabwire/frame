package connect

import (
	"context"

	"connectrpc.com/connect/v2"

	"github.com/pitabwire/frame/v3/security/authorizer"
)

// functionAccessInterceptor is a Connect interceptor that automatically enforces
// functional permissions based on a pre-built mapping of RPC procedure names to
// required permission strings. It uses FunctionChecker to verify each required
// permission against the authorization backend (e.g., Ory Keto).
//
// The permission map keys are Connect procedure names (e.g.,
// "/profile.v1.ProfileService/GetById") and values are slices of permission
// strings that must ALL be satisfied (AND logic).
//
// This interceptor should be placed after the authentication and tenancy access
// interceptors in the chain so that claims are available in the context.
type functionAccessInterceptor struct {
	checker     *authorizer.FunctionChecker
	permissions map[string][]string
}

// NewFunctionAccessInterceptor creates a Connect interceptor that enforces
// functional permissions automatically based on a procedure-to-permissions map.
//
// The permissions map should be keyed by Connect procedure name (e.g.,
// "/profile.v1.ProfileService/GetById") with values being the permission
// strings required for that procedure. Use the permissions.BuildProcedureMap
// helper from the apis/go/common/permissions package to build this map from
// proto service descriptors.
//
// If a procedure is not in the map, the request is allowed through without
// a functional permission check.
func NewFunctionAccessInterceptor(
	checker *authorizer.FunctionChecker,
	permissions map[string][]string,
) connect.ServerInterceptor {
	f := &functionAccessInterceptor{
		checker:     checker,
		permissions: permissions,
	}
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			if err := f.checkPermissions(ctx, spec.Procedure); err != nil {
				return err
			}
			return next(ctx, spec, stream)
		}
	}
}

func (f *functionAccessInterceptor) checkPermissions(ctx context.Context, procedure string) error {
	perms, ok := f.permissions[procedure]
	if !ok || len(perms) == 0 {
		return nil
	}

	for _, perm := range perms {
		if err := f.checker.Check(ctx, perm); err != nil {
			return authorizer.ToConnectError(err)
		}
	}

	return nil
}
