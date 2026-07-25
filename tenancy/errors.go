package tenancy

import "errors"

// Secure-mode acquire errors. Machines and interceptors should treat these
// as "not ready / not permitted to access storage" — map to FailedPrecondition
// on RPC paths (not PermissionDenied, which is reserved for ReBAC).
var (
	// ErrClaimsRequired is returned in secure modes when no bindable
	// claims or SystemPrincipal is present on the context.
	ErrClaimsRequired = errors.New("tenancy: claims required")

	// ErrTenantIDRequired is returned when binding would be partition-only
	// (empty TenantID) or a scoped SystemPrincipal lacks TenantID.
	ErrTenantIDRequired = errors.New("tenancy: tenant id required for binding")

	// ErrSkipNotPermitted is returned when Claims.Skip is set without a
	// SystemPrincipal in secure modes. Bare WithSkipEnforcement is
	// FailOpen-only.
	ErrSkipNotPermitted = errors.New("tenancy: skip not permitted without system principal")

	// ErrAllowGlobalDenied is returned when SystemPrincipal.AllowGlobal is
	// set without the unexported framework migration marker and without
	// an allowlisted ServiceName on the provider.
	ErrAllowGlobalDenied = errors.New("tenancy: AllowGlobal not authorized for this service")

	// ErrInvalidCacheKeySegment is returned when a tenant/service segment
	// cannot be safely used as a cache key prefix.
	ErrInvalidCacheKeySegment = errors.New("tenancy: invalid cache key segment")

	// ErrCacheTenantRequired is returned when a tenant-aware cache is used
	// in a secure mode without a bindable tenant.
	ErrCacheTenantRequired = errors.New("tenancy: tenant required for cache key")
)
