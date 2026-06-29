package tenancy

import (
	"context"

	"github.com/pitabwire/frame/v2/security"
)

// Claims is the storage-layer view of a principal's tenancy. Treat as
// immutable: every transformation returns a new instance.
type Claims struct {
	// TenantID is the single tenant this principal belongs to.
	TenantID string

	// PartitionIDs are every partition this principal can access. One
	// principal may legitimately span multiple partitions (e.g., an
	// operator with access to several branches, an analyst aggregating
	// across groups). Single-partition principals carry one element.
	PartitionIDs []string

	// AccessID is an optional access-control hint propagated through
	// queue metadata and lifecycle hooks.
	AccessID string

	// Skip is true for internal/system callers that should bypass
	// tenancy enforcement. Providers honour Skip by performing no
	// session binding for the conn — the database-side policy's
	// empty-match-all branch then keeps every row visible.
	Skip bool
}

// IsEmpty reports whether the claims carry enforceable tenancy. Empty
// claims behave identically to "no claims attached" from a provider's
// perspective.
func (c *Claims) IsEmpty() bool {
	if c == nil {
		return true
	}
	return c.TenantID == "" && len(c.PartitionIDs) == 0
}

// ExtendPartitions returns a new Claims with the supplied partition IDs
// merged in. Preserves TenantID, AccessID, and Skip unchanged. Empty
// strings are ignored; duplicates are removed; existing order is kept
// and new IDs appended after.
//
// A nil receiver yields a fresh Claims carrying only the deduplicated
// non-empty partition IDs; TenantID, AccessID, and Skip default to
// zero values in that path.
func (c *Claims) ExtendPartitions(partitionIDs ...string) *Claims {
	if c == nil {
		return &Claims{PartitionIDs: dedupedNonEmpty(partitionIDs)}
	}

	merged := make([]string, 0, len(c.PartitionIDs)+len(partitionIDs))
	seen := make(map[string]struct{}, cap(merged))
	for _, p := range c.PartitionIDs {
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}
	for _, p := range partitionIDs {
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}

	return &Claims{
		TenantID:     c.TenantID,
		PartitionIDs: merged,
		AccessID:     c.AccessID,
		Skip:         c.Skip,
	}
}

func dedupedNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// claimsKey is the unexported context key under which Claims are
// stored. Using an unexported empty struct prevents collisions with
// other packages' context values.
type claimsKey struct{}

// WithClaims binds Claims to ctx. Returns the parent ctx unchanged
// when c is nil to avoid hiding a "no claims" signal behind a
// non-empty context.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, claimsKey{}, c)
}

// ClaimsFromContext returns the bound Claims with graceful fallback:
//
//  1. Explicit Claims bound via WithClaims (fastest path).
//  2. Derived from security.AuthenticationClaims if present in ctx
//     (job workers / services that haven't run the tenancy interceptor
//     still get correct enforcement).
//  3. nil — caller is unscoped (system services, migrations).
func ClaimsFromContext(ctx context.Context) *Claims {
	if v, ok := ctx.Value(claimsKey{}).(*Claims); ok {
		return v
	}
	if auth := security.ClaimsFromContext(ctx); auth != nil {
		return ClaimsFromAuth(ctx, auth)
	}
	return nil
}

// ClaimsFromAuth derives Claims from auth claims using the frame
// default mapping:
//
//	TenantID     = auth.GetTenantID()
//	PartitionIDs = auth.GetPartitionIDs()
//	AccessID     = auth.GetAccessID()
//	Skip         = auth.IsInternalSystem() || security.IsTenancyChecksOnClaimSkipped(ctx)
//
// Not overridable — callers needing different semantics build Claims
// directly and bind via WithClaims.
func ClaimsFromAuth(ctx context.Context, auth *security.AuthenticationClaims) *Claims {
	if auth == nil {
		return nil
	}
	return &Claims{
		TenantID:     auth.GetTenantID(),
		PartitionIDs: auth.GetPartitionIDs(),
		AccessID:     auth.GetAccessID(),
		Skip:         auth.IsInternalSystem() || security.IsTenancyChecksOnClaimSkipped(ctx),
	}
}

// WithExtraPartitions reads the current Claims from ctx, extends them
// with the supplied partition IDs (preserving TenantID, AccessID, Skip),
// and binds the extended Claims to a child ctx. Returns ctx unchanged
// when no claims are present.
//
// Use for service-on-behalf-of flows, cross-branch reporting, or any
// case where a principal legitimately needs visibility over additional
// partitions without changing tenant.
func WithExtraPartitions(ctx context.Context, partitionIDs ...string) context.Context {
	current := ClaimsFromContext(ctx)
	if current == nil {
		return ctx
	}
	extended := current.ExtendPartitions(partitionIDs...)
	return WithClaims(ctx, extended)
}

// WithSkipEnforcement returns a context that bypasses tenancy
// enforcement for any database query made through it. Use for
// migration scripts, admin tools, or system-level operations that
// legitimately need full-table access.
//
// Internally this binds a Claims value with Skip=true. Providers
// honour Skip by performing no session binding for the connection,
// which makes the database-side policy's empty-match-all branch fire
// — i.e. every row is visible.
func WithSkipEnforcement(ctx context.Context) context.Context {
	return WithClaims(ctx, &Claims{Skip: true})
}
