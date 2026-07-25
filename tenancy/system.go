package tenancy

import (
	"context"
	"strings"
)

// SystemPrincipal elevates process-local work (jobs, admin exports) without
// relying on ambient roles=internal Skip. Prefer scoped TenantID over
// AllowGlobal. Reason is for logs/metrics only — never authorization.
//
// For service-to-service "on behalf of tenant" RPCs, continue to use
// security.EnrichTenancyClaims with internal JWTs (binds RLS, no Skip under
// Secure Profile). SystemPrincipal is for process-local elevation.
type SystemPrincipal struct {
	ServiceName  string
	Reason       string // logs/metrics ONLY — never used for authorization
	TenantID     string // if set and !AllowGlobal, beforeAcquire binds this scope
	PartitionIDs []string
	AllowGlobal  bool // match-all; requires provider allowlist OR framework marker
}

type systemPrincipalKey struct{}

// WithSystemPrincipal binds a SystemPrincipal to ctx. The principal is
// normalized (trimmed IDs, deduped partitions).
func WithSystemPrincipal(ctx context.Context, p SystemPrincipal) context.Context {
	p.ServiceName = strings.TrimSpace(p.ServiceName)
	p.Reason = strings.TrimSpace(p.Reason)
	p.TenantID = strings.TrimSpace(p.TenantID)
	p.PartitionIDs = normalizePartitionIDs(p.PartitionIDs)
	return context.WithValue(ctx, systemPrincipalKey{}, p)
}

// SystemPrincipalFromContext returns the bound SystemPrincipal if any.
func SystemPrincipalFromContext(ctx context.Context) (SystemPrincipal, bool) {
	if ctx == nil {
		return SystemPrincipal{}, false
	}
	p, ok := ctx.Value(systemPrincipalKey{}).(SystemPrincipal)
	return p, ok
}

// IsSystemPrincipal reports whether ctx carries a SystemPrincipal.
func IsSystemPrincipal(ctx context.Context) bool {
	_, ok := SystemPrincipalFromContext(ctx)
	return ok
}
