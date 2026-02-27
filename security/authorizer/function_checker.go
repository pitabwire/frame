package authorizer

import (
	"context"
	"fmt"

	"github.com/pitabwire/frame/security"
)

// FunctionCheckerOption configures a FunctionChecker.
type FunctionCheckerOption func(*FunctionChecker)

// WithFunctionSubjectNamespace overrides the default subject namespace.
func WithFunctionSubjectNamespace(ns string) FunctionCheckerOption {
	return func(c *FunctionChecker) {
		c.subjectNamespace = ns
	}
}

// FunctionChecker verifies functional permissions in application-specific
// namespaces (e.g., service_tenancy, service_payment). It extracts tenant and
// partition from the caller's claims and checks whether the caller has a
// specific permission in the configured namespace.
//
// Unlike TenancyAccessChecker, FunctionChecker has no provisioning callback —
// it performs a pure permission check. Data access should be verified
// separately using TenancyAccessChecker before calling FunctionChecker.
type FunctionChecker struct {
	authorizer       security.Authorizer
	objectNamespace  string
	subjectNamespace string
}

// NewFunctionChecker creates a checker that verifies functional permissions
// against the given objectNamespace.
func NewFunctionChecker(
	auth security.Authorizer,
	objectNamespace string,
	opts ...FunctionCheckerOption,
) *FunctionChecker {
	c := &FunctionChecker{
		authorizer:       auth,
		objectNamespace:  objectNamespace,
		subjectNamespace: security.NamespaceProfile,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Check verifies that the caller in ctx has the given permission on the
// tenant/partition identified in their claims.
func (c *FunctionChecker) Check(ctx context.Context, permission string) error {
	claims := security.ClaimsFromContext(ctx)
	if claims == nil {
		return ErrInvalidSubject
	}

	subjectID, err := claims.GetSubject()
	if err != nil || subjectID == "" {
		return ErrInvalidSubject
	}

	tenantID := claims.GetTenantID()
	if tenantID == "" {
		return ErrInvalidObject
	}

	partitionID := claims.GetPartitionID()
	if partitionID == "" {
		return ErrInvalidObject
	}

	tenancyPath := fmt.Sprintf("%s/%s", tenantID, partitionID)

	req := security.CheckRequest{
		Object:     security.ObjectRef{Namespace: c.objectNamespace, ID: tenancyPath},
		Permission: permission,
		Subject:    security.SubjectRef{Namespace: c.subjectNamespace, ID: subjectID},
	}

	result, err := c.authorizer.Check(ctx, req)
	if err != nil {
		return err
	}

	if result.Allowed {
		return nil
	}

	return NewPermissionDeniedError(req.Object, permission, req.Subject, result.Reason)
}
