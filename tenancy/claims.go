package tenancy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pitabwire/frame/v2/security"
)

// Sentinel errors for claims validation.
var (
	// ErrInvalidTenantID is returned when a tenant id contains reserved
	// characters or is otherwise unusable for storage-layer binding.
	ErrInvalidTenantID = errors.New("tenancy: invalid tenant id")

	// ErrInvalidPartitionID is returned when a partition id contains the
	// reserved list separator ',' (used by the Postgres session GUC CSV
	// encoding) or is otherwise unusable.
	ErrInvalidPartitionID = errors.New("tenancy: invalid partition id")
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

	// AccessID is the membership grant used for this session (write
	// attribution). Stamped onto rows at create; not used for RLS or Keto.
	// Empty is allowed (service accounts, system jobs).
	AccessID string

	// Skip is true for internal/system callers that should bypass
	// tenancy enforcement. Providers honour Skip by not binding session
	// scope — the database-side policy's empty-match-all branch then
	// keeps every row visible (same as missing claims).
	Skip bool
}

// IsEmpty reports whether the claims carry enforceable tenancy. Empty
// claims behave identically to "no claims attached" from a provider's
// perspective (no filtering, no error). Whitespace-only IDs are treated
// as empty (call Normalize first for full sanitisation).
func (c *Claims) IsEmpty() bool {
	if c == nil {
		return true
	}
	if strings.TrimSpace(c.TenantID) != "" {
		return false
	}
	for _, p := range c.PartitionIDs {
		if strings.TrimSpace(p) != "" {
			return false
		}
	}
	return true
}

// Normalize returns a copy with trimmed IDs, empty partitions dropped,
// and partition IDs deduplicated (order preserved). A nil receiver yields
// nil. The receiver is never mutated.
func (c *Claims) Normalize() *Claims {
	if c == nil {
		return nil
	}
	return &Claims{
		TenantID:     strings.TrimSpace(c.TenantID),
		PartitionIDs: normalizePartitionIDs(c.PartitionIDs),
		AccessID:     strings.TrimSpace(c.AccessID),
		Skip:         c.Skip,
	}
}

// IsBindable reports whether claims can drive RLS session binding:
// non-nil, not Skip, non-empty, and with a non-empty TenantID after trim.
// Partition-only claims are not bindable under Secure Profile rules.
func (c *Claims) IsBindable() bool {
	if c == nil || c.Skip || c.IsEmpty() {
		return false
	}
	return strings.TrimSpace(c.TenantID) != ""
}

// Validate reports whether the claims are safe to bind to a storage
// session. Empty claims and Skip claims always pass (providers do not
// bind session scope for them). Non-empty claims must have well-formed
// tenant and partition identifiers.
//
// Rules:
//   - tenant id must not contain ','
//   - no partition id may contain ',' (CSV separator for session GUCs)
//
// Call Normalize before Validate when inputs may carry surrounding
// whitespace.
func (c *Claims) Validate() error {
	if c == nil || c.Skip || c.IsEmpty() {
		return nil
	}
	if err := validateID(c.TenantID, ErrInvalidTenantID); err != nil {
		return err
	}
	return ValidatePartitionIDs(c.PartitionIDs)
}

// ValidatePartitionIDs rejects partition IDs that would corrupt the CSV
// encoding used by storage providers (string_to_array on ','). Empty
// strings are ignored. Exported so providers can validate without
// reimplementing the rule.
func ValidatePartitionIDs(ids []string) error {
	for _, id := range ids {
		if id == "" {
			continue
		}
		if err := validateID(id, ErrInvalidPartitionID); err != nil {
			return err
		}
	}
	return nil
}

func validateID(id string, sentinel error) error {
	if id == "" {
		return nil
	}
	if strings.Contains(id, ",") {
		return fmt.Errorf("%w: %q contains ',' which is reserved as the list separator", sentinel, id)
	}
	return nil
}

func normalizePartitionIDs(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ExtendPartitions returns a new Claims with the supplied partition IDs
// merged in. Preserves TenantID, AccessID, and Skip unchanged. Empty
// strings are ignored; duplicates are removed; existing order is kept
// and new IDs appended after. IDs are trimmed.
//
// A nil receiver yields a fresh Claims carrying only the deduplicated
// non-empty partition IDs; TenantID, AccessID, and Skip default to
// zero values in that path.
func (c *Claims) ExtendPartitions(partitionIDs ...string) *Claims {
	if c == nil {
		return &Claims{PartitionIDs: normalizePartitionIDs(partitionIDs)}
	}

	merged := make([]string, 0, len(c.PartitionIDs)+len(partitionIDs))
	seen := make(map[string]struct{}, cap(merged))
	appendUnique := func(ids []string) {
		for _, p := range ids {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, dup := seen[p]; dup {
				continue
			}
			seen[p] = struct{}{}
			merged = append(merged, p)
		}
	}
	appendUnique(c.PartitionIDs)
	appendUnique(partitionIDs)

	return &Claims{
		TenantID:     strings.TrimSpace(c.TenantID),
		PartitionIDs: merged,
		AccessID:     strings.TrimSpace(c.AccessID),
		Skip:         c.Skip,
	}
}

// claimsKey is the unexported context key under which Claims are
// stored. Using an unexported empty struct prevents collisions with
// other packages' context values.
type claimsKey struct{}

// WithClaims binds Claims to ctx. Nil claims leave ctx unchanged.
// Non-nil claims are normalized before binding so storage providers
// never see whitespace-padded IDs.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, claimsKey{}, c.Normalize())
}

// ClaimsFromContext returns the bound Claims with graceful fallback:
//
//  1. Explicit Claims bound via WithClaims (fastest path).
//  2. Derived from security.AuthenticationClaims if present in ctx
//     (job workers / services that haven't run the tenancy interceptor
//     still get correct enforcement).
//  3. nil — no tenancy in context; provider does not filter (and does
//     not error). Use WithSkipEnforcement or omit claims for full-table
//     access; bind claims when you want RLS filtering.
func ClaimsFromContext(ctx context.Context) *Claims {
	if v, ok := ctx.Value(claimsKey{}).(*Claims); ok {
		return v
	}
	if auth := security.ClaimsFromContext(ctx); auth != nil {
		return ClaimsFromAuth(ctx, auth)
	}
	return nil
}

// ClaimsFromAuthOption configures ClaimsFromAuth. Options are applied
// per call — no package globals.
type ClaimsFromAuthOption func(*claimsFromAuthConfig)

type claimsFromAuthConfig struct {
	// HonorInternalSkip defaults true for back-compat when no options
	// are passed. Secure Profile binders pass WithHonorInternalSkip(false).
	HonorInternalSkip bool
}

// WithHonorInternalSkip controls whether roles=internal and
// SkipTenancyChecksOnClaims map to Claims.Skip.
func WithHonorInternalSkip(v bool) ClaimsFromAuthOption {
	return func(c *claimsFromAuthConfig) {
		c.HonorInternalSkip = v
	}
}

// ClaimsFromAuth derives Claims from auth claims using the frame
// default mapping:
//
//	TenantID     = auth.GetTenantID()
//	PartitionIDs = auth.GetPartitionIDs()
//	AccessID     = auth.GetAccessID()
//	Skip         = (optional) auth.IsInternalSystem() || IsTenancyChecksOnClaimSkipped
//
// The result is always normalized (trimmed, deduped partitions).
//
// Default (no opts): HonorInternalSkip=true for back-compat.
// Secure Profile: ClaimsFromAuth(ctx, auth, WithHonorInternalSkip(false)).
func ClaimsFromAuth(ctx context.Context, auth *security.AuthenticationClaims, opts ...ClaimsFromAuthOption) *Claims {
	if auth == nil {
		return nil
	}
	cfg := claimsFromAuthConfig{HonorInternalSkip: true}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	// Skip is driven by the context flag set in ClaimsToContext for internal
	// callers (legacy). WithoutInternalTenancySkip leaves the flag unset so
	// internal JWTs still bind RLS. HonorInternalSkip=false forces Skip off
	// even if the flag was set (Secure Profile / TrustTenancyOnly).
	skip := false
	if cfg.HonorInternalSkip {
		skip = security.IsTenancyChecksOnClaimSkipped(ctx)
	}
	return (&Claims{
		TenantID:     auth.GetTenantID(),
		PartitionIDs: auth.GetPartitionIDs(),
		AccessID:     auth.GetAccessID(),
		Skip:         skip,
	}).Normalize()
}

// ClaimsFromContextWithOptions is like ClaimsFromContext but uses the
// supplied ClaimsFromAuth options when falling back to auth claims.
func ClaimsFromContextWithOptions(ctx context.Context, opts ...ClaimsFromAuthOption) *Claims {
	if v, ok := ctx.Value(claimsKey{}).(*Claims); ok {
		return v
	}
	if auth := security.ClaimsFromContext(ctx); auth != nil {
		return ClaimsFromAuth(ctx, auth, opts...)
	}
	return nil
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
// Internally this binds a Claims value with Skip=true. Providers do not
// bind session scope for Skip (same as missing claims): every row is
// visible. Prefer this over relying on "no claims" when you want the
// bypass to be explicit in call sites.
func WithSkipEnforcement(ctx context.Context) context.Context {
	return WithClaims(ctx, &Claims{Skip: true})
}
