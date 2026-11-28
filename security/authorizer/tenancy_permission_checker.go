package authorizer

import (
	"context"
	"fmt"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/v3/security"
)

// TenancyAccessDeniedFunc is called when a system_internal caller is denied permission.
// It should provision the necessary tuples so that a retry succeeds.
type TenancyAccessDeniedFunc func(ctx context.Context, auth security.Authorizer, tenantID, subjectID string) error

// PartitionVerifierFunc reports whether tenantID/partitionID is a real,
// registered partition. It gates self-healing for internal callers: the
// tenancy a caller names in request headers is unverified input, so
// provisioning must never run for a partition that does not exist.
// Returning an error fails closed (no self-heal).
type PartitionVerifierFunc func(ctx context.Context, tenantID, partitionID string) (bool, error)

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

// WithPartitionVerifier replaces the default partition-existence check used
// before self-healing an internal caller's denial for a partition that its
// token does not cover. The default treats a partition as existing when the
// object namespace already holds at least one tuple for it, which the
// tenancy service writes when it registers a partition. Services with direct
// access to the partition registry should supply an authoritative verifier.
func WithPartitionVerifier(fn PartitionVerifierFunc) TenantPermissionCheckerOption {
	return func(c *TenancyAccessChecker) {
		c.verifyPartition = fn
	}
}

// WithTenancyConstraints adds contextual constraints evaluated after the
// Keto relation check passes.
func WithTenancyConstraints(constraints ...AccessConstraint) TenantPermissionCheckerOption {
	return func(c *TenancyAccessChecker) {
		c.constraints = append(c.constraints, constraints...)
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
	verifyPartition       PartitionVerifierFunc
	constraints           []AccessConstraint
}

// NewTenancyAccessChecker creates a checker that verifies permissions
// against objectNamespace using the provided authorizer.
//
// By default, when an internal system caller (roles include "internal") is
// denied Plane-1 access, the checker self-heals by writing
// `{objectNamespace}:{path}#service ← profile_user:{profileID}` and retries
// once. Regular user denials only log. Override with WithOnTenancyAccessDenied.
//
// Self-healing (default or custom callback) only runs for an internal caller
// when the target partition is either covered by the caller's authenticated
// token claims, or verified to exist (see WithPartitionVerifier). Internal
// callers may name any tenancy in X-Tenant-Id / X-Partition-Id headers, so
// without this gate a bot could mint access to arbitrary, non-existent
// partitions.
func NewTenancyAccessChecker(
	auth security.Authorizer,
	objectNamespace string,
	opts ...TenantPermissionCheckerOption,
) *TenancyAccessChecker {
	c := &TenancyAccessChecker{
		authorizer:       auth,
		objectNamespace:  objectNamespace,
		subjectNamespace: security.NamespaceProfile,
	}
	// Default after struct fields are set so the closure can use c.objectNamespace.
	c.onTenancyAccessDenied = c.defaultOnTenancyAccessDenied
	c.verifyPartition = c.partitionKnownToAuthorizer
	for _, o := range opts {
		o(c)
	}
	return c
}

// defaultOnTenancyAccessDenied self-heals missing Plane-1 #service tuples for
// internal service bots. Non-internal callers are logged only (no grant).
func (c *TenancyAccessChecker) defaultOnTenancyAccessDenied(
	ctx context.Context,
	auth security.Authorizer,
	tenancyPath, subjectID string,
) error {
	fields := map[string]any{
		"tenant_id":          tenancyPath,
		fieldSubjectID:       subjectID,
		fieldObjectNamespace: c.objectNamespace,
	}

	claims := security.ClaimsFromContext(ctx)
	if claims == nil || !claims.IsInternalSystem() {
		util.Log(ctx).WithFields(fields).Error("PERMISSION DENIED: tenancy access denied")
		return nil
	}

	tuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: c.objectNamespace, ID: tenancyPath},
		Relation: "service",
		Subject:  security.SubjectRef{Namespace: c.subjectNamespace, ID: subjectID},
	}
	if err := auth.WriteTuple(ctx, tuple); err != nil {
		util.Log(ctx).WithFields(fields).WithError(err).
			Error("PERMISSION DENIED: self-heal of tenancy service access failed")
		return err
	}
	util.Log(ctx).WithFields(fields).
		Info("self-healed missing tenancy service tuple for internal caller")
	return nil
}

// partitionKnownToAuthorizer is the default PartitionVerifierFunc: a
// partition exists when the object namespace already holds at least one
// tuple for it (members, parent links or service grants written by the
// tenancy service when the partition was registered).
func (c *TenancyAccessChecker) partitionKnownToAuthorizer(
	ctx context.Context, tenantID, partitionID string,
) (bool, error) {
	tuples, err := c.authorizer.ListRelations(ctx, security.ObjectRef{
		Namespace: c.objectNamespace,
		ID:        fmt.Sprintf("%s/%s", tenantID, partitionID),
	})
	if err != nil {
		return false, err
	}
	return len(tuples) > 0, nil
}

// tokenCoversPartition reports whether the caller's authenticated token
// (not header-supplied tenancy) was issued for tenantID/partitionID.
func tokenCoversPartition(ctx context.Context, tenantID, partitionID string) bool {
	token := security.TokenClaimsFromContext(ctx)
	if token == nil || token.GetTenantID() != tenantID {
		return false
	}
	for _, p := range token.GetPartitionIDs() {
		if p == partitionID {
			return true
		}
	}
	return false
}

// mayProvision decides whether the self-heal callback may run for this
// denial. Non-internal callers are passed through (the default callback only
// logs for them). Internal callers are allowed only for partitions their
// token covers or that the verifier confirms exist; anything else fails
// closed.
func (c *TenancyAccessChecker) mayProvision(
	ctx context.Context, claims *security.AuthenticationClaims, tenantID, partitionID string,
) bool {
	if !claims.IsInternalSystem() || tokenCoversPartition(ctx, tenantID, partitionID) {
		return true
	}

	fields := map[string]any{
		"tenant_id":          tenantID,
		"partition_id":       partitionID,
		fieldSubjectID:       claims.GetProfileID(),
		fieldObjectNamespace: c.objectNamespace,
	}
	if c.verifyPartition == nil {
		util.Log(ctx).WithFields(fields).
			Error("PERMISSION DENIED: no partition verifier; refusing to self-heal header-supplied tenancy")
		return false
	}
	exists, err := c.verifyPartition(ctx, tenantID, partitionID)
	if err != nil {
		util.Log(ctx).WithFields(fields).WithError(err).
			Error("PERMISSION DENIED: could not verify partition; refusing to self-heal")
		return false
	}
	if !exists {
		util.Log(ctx).WithFields(fields).
			Error("PERMISSION DENIED: refusing to self-heal access to an unknown partition")
		return false
	}
	return true
}

// CheckAccess verifies that the caller has data access to the partition
// identified in their claims. For regular users it checks the "member"
// relation; for internal system callers it checks the "service" relation.
// Service accounts are expected to have a "service" tuple on the root
// partition, with child partitions inheriting access via SubjectSet chains.
// If denied and a self-healing callback is set, it invokes the callback
// and retries once.
func (c *TenancyAccessChecker) CheckAccess(ctx context.Context) error {
	claims := security.ClaimsFromContext(ctx)
	if claims == nil {
		return ErrInvalidSubject
	}

	// Actor for ReBAC is always the profile, never the OAuth2 client_id.
	// JWT sub may be client_id for client_credentials; profile_id claim wins.
	subjectID := claims.GetProfileID()
	if subjectID == "" {
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

	relation := "member"
	if claims.IsInternalSystem() {
		relation = "service"
	}

	req := security.CheckRequest{
		Object:     security.ObjectRef{Namespace: c.objectNamespace, ID: tenancyPath},
		Permission: relation,
		Subject:    security.SubjectRef{Namespace: c.subjectNamespace, ID: subjectID},
	}

	result, err := c.authorizer.Check(ctx, req)
	if err != nil {
		return err
	}

	if result.Allowed {
		return evaluateConstraints(ctx, c.constraints, relation, req.Object, req.Subject)
	}

	// Self-healing: invoke callback and retry once.
	if c.onTenancyAccessDenied != nil && c.mayProvision(ctx, claims, tenantID, partitionID) {
		if provisionErr := c.onTenancyAccessDenied(ctx, c.authorizer, tenancyPath, subjectID); provisionErr != nil {
			return NewPermissionDeniedError(req.Object, relation, req.Subject, result.Reason)
		}

		result, err = c.authorizer.Check(ctx, req)
		if err != nil {
			return err
		}
		if result.Allowed {
			return evaluateConstraints(ctx, c.constraints, relation, req.Object, req.Subject)
		}
	}

	return NewPermissionDeniedError(req.Object, relation, req.Subject, result.Reason)
}

// Check verifies that the caller in ctx has the given permission on the
// tenant identified in their claims.
func (c *TenancyAccessChecker) Check(ctx context.Context, permission string) error {
	claims := security.ClaimsFromContext(ctx)
	if claims == nil {
		return ErrInvalidSubject
	}

	subjectID := claims.GetProfileID()
	if subjectID == "" {
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
		return evaluateConstraints(ctx, c.constraints, permission, req.Object, req.Subject)
	}

	if c.onTenancyAccessDenied != nil && c.mayProvision(ctx, claims, tenantID, partitionID) {
		if provisionErr := c.onTenancyAccessDenied(ctx, c.authorizer, tenancyPath, subjectID); provisionErr != nil {
			return NewPermissionDeniedError(req.Object, permission, req.Subject, result.Reason)
		}

		// Retry after provisioning.
		result, err = c.authorizer.Check(ctx, req)
		if err != nil {
			return err
		}
		if result.Allowed {
			return evaluateConstraints(ctx, c.constraints, permission, req.Object, req.Subject)
		}
	}

	return NewPermissionDeniedError(req.Object, permission, req.Subject, result.Reason)
}
