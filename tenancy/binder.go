package tenancy

import (
	"context"
	"fmt"

	"github.com/pitabwire/frame/v2/security"
)

// ClaimsBinder derives and binds storage-layer tenancy.Claims for request
// paths. Construct once at service start from Secure Profile options — do
// not use package globals.
type ClaimsBinder struct {
	// HonorInternalSkip when true (default) maps roles=internal and
	// SkipTenancyChecksOnClaims to Claims.Skip. Secure Profile sets false.
	HonorInternalSkip bool

	// RequireClaims when true fails Bind early if no bindable tenancy is
	// present after auth (Secure Profile / Hybrid interceptors).
	RequireClaims bool
}

// NewClaimsBinder returns a binder with Secure Profile defaults when
// secure is true (HonorInternalSkip=false, RequireClaims=true).
// Legacy defaults: HonorInternalSkip=true, RequireClaims=false.
func NewClaimsBinder(secure bool) *ClaimsBinder {
	if secure {
		return &ClaimsBinder{HonorInternalSkip: false, RequireClaims: true}
	}
	return &ClaimsBinder{HonorInternalSkip: true, RequireClaims: false}
}

// Bind derives tenancy claims from auth context and binds them.
// Returns ErrClaimsRequired when RequireClaims is set and no bindable
// claims result (and no SystemPrincipal).
func (b *ClaimsBinder) Bind(ctx context.Context) (context.Context, error) {
	if b == nil {
		b = NewClaimsBinder(false)
	}
	auth := security.ClaimsFromContext(ctx)
	if auth != nil {
		opts := []ClaimsFromAuthOption{WithHonorInternalSkip(b.HonorInternalSkip)}
		claims := ClaimsFromAuth(ctx, auth, opts...)
		if claims != nil {
			ctx = WithClaims(ctx, claims)
		}
	}

	if !b.RequireClaims {
		return ctx, nil
	}
	// SystemPrincipal (scoped or global) satisfies require-claims for jobs.
	if IsSystemPrincipal(ctx) {
		return ctx, nil
	}
	claims := ClaimsFromContext(ctx)
	if claims != nil && claims.IsBindable() {
		return ctx, nil
	}
	// Skip alone is not bindable in secure modes.
	if claims != nil && claims.Skip {
		return ctx, fmt.Errorf("%w: skip without system principal", ErrClaimsRequired)
	}
	return ctx, ErrClaimsRequired
}
