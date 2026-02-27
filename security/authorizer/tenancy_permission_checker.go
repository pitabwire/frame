package authorizer

import (
	"context"
	"fmt"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/security"
)

// TenancyAccessDeniedFunc is called when a system_internal caller is denied permission.
// It should provision the necessary tuples so that a retry succeeds.
type TenancyAccessDeniedFunc func(ctx context.Context, auth security.Authorizer, tenantID, subjectID string) error

// TenantPermissionCheckerOption configures a TenancyAccessChecker.
type TenantPermissionCheckerOption func(*TenancyAccessChecker)

// WithSubjectNamespace overrides the default subject namespace (security.NamespaceProfile).
func WithSubjectNamespace(ns string) TenantPermissionCheckerOption {
	return func(c *TenancyAccessChecker) {
		c.subjectNamespace = ns
	}
}

// WithOnTenancyAccessDenied registers a callback invoked when a system_internal
// caller is denied. The callback should provision the required tuples so that
// a subsequent retry can succeed.
func WithOnTenancyAccessDenied(fn TenancyAccessDeniedFunc) TenantPermissionCheckerOption {
	return func(c *TenancyAccessChecker) {
		c.onTenancyAccessDenied = fn
	}
}

// TenancyAccessChecker extracts claims from context, builds a CheckRequest
// against a tenant-scoped object namespace, and calls the authorizer. For
// system_internal callers it supports a self-healing callback that provisions
// missing tuples and retries.
type TenancyAccessChecker struct {
	authorizer            security.Authorizer
	objectNamespace       string
	subjectNamespace      string
	onTenancyAccessDenied TenancyAccessDeniedFunc
}

// NewTenancyAccessChecker creates a checker that verifies permissions
// against objectNamespace using the provided authorizer.
func NewTenancyAccessChecker(
	auth security.Authorizer,
	objectNamespace string,
	opts ...TenantPermissionCheckerOption,
) *TenancyAccessChecker {
	c := &TenancyAccessChecker{
		authorizer:       auth,
		objectNamespace:  objectNamespace,
		subjectNamespace: security.NamespaceProfile,
		onTenancyAccessDenied: func(ctx context.Context, _ security.Authorizer, tenancyPath, subjectID string) error {
			util.Log(ctx).WithFields(map[string]any{
				"tenant_id":  tenancyPath,
				"subject_id": subjectID,
			}).Error("PERMISSION DENIED: tenancy access denied")
			return nil
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Check verifies that the caller in ctx has the given permission on the
// tenant identified in their claims.
func (c *TenancyAccessChecker) Check(ctx context.Context, permission string) error {
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

	if c.onTenancyAccessDenied != nil {
		if provisionErr := c.onTenancyAccessDenied(ctx, c.authorizer, tenancyPath, subjectID); provisionErr != nil {
			return NewPermissionDeniedError(req.Object, permission, req.Subject, result.Reason)
		}

		// Retry after provisioning.
		result, err = c.authorizer.Check(ctx, req)
		if err != nil {
			return err
		}
		if !result.Allowed {
			return NewPermissionDeniedError(req.Object, permission, req.Subject, result.Reason)
		}
		return nil
	}

	return NewPermissionDeniedError(req.Object, permission, req.Subject, result.Reason)
}
