package frame

import (
	"context"
	"os"
	"strings"

	"github.com/pitabwire/frame/v2/queue/protocol"
	"github.com/pitabwire/frame/v2/tenancy"
)

// WithTenancySecurityMode sets ModeFailOpen (default), ModeHybrid, or ModeFailClosed.
// Env FRAME_TENANCY_SECURITY_MODE is applied when constructing the default
// provider if this option is not used (see WithDatastore).
func WithTenancySecurityMode(m tenancy.SecurityMode) Option {
	return func(_ context.Context, s *Service) {
		s.tenancySecurityMode = m
	}
}

// WithSystemPrincipalAllowGlobal allowlists ServiceName values that may use
// SystemPrincipal.AllowGlobal for match-all elevation in secure modes.
func WithSystemPrincipalAllowGlobal(serviceNames ...string) Option {
	return func(_ context.Context, s *Service) {
		s.allowGlobalServices = append([]string{}, serviceNames...)
	}
}

// WithLegacyInternalSkip controls whether roles=internal maps to tenancy Skip.
// Default true. Secure Profile uses false so internal JWTs bind RLS instead of
// match-all.
func WithLegacyInternalSkip(honor bool) Option {
	return func(_ context.Context, s *Service) {
		s.legacyInternalSkip = honor
		s.legacyInternalSkipSet = true
	}
}

// WithQueueClaimTrust sets pull/push reconstruction trust for queue consumers.
// Secure Profile: protocol.TrustTenancyOnly. Default: protocol.TrustAll.
func WithQueueClaimTrust(level protocol.ClaimTrustLevel) Option {
	return func(_ context.Context, s *Service) {
		v := int(level)
		s.queueClaimTrust = &v
	}
}

// WithEnrollmentStrict fails migrate when models look tenanted but do not
// implement tenancy.Tenanted (and are not Unscoped).
func WithEnrollmentStrict(strict bool) Option {
	return func(_ context.Context, s *Service) {
		s.enrollmentStrict = strict
	}
}

// WithTenancyArmingCheck enables/disables the readiness check that the app DB
// role is not superuser/BYPASSRLS. When unset, the check is auto-enabled if
// SecurityMode is Hybrid or FailClosed.
func WithTenancyArmingCheck(enabled bool) Option {
	return func(_ context.Context, s *Service) {
		s.tenancyArmingCheck = &enabled
	}
}

// WithCacheTenantPrefix enables transparent t/sys/g/ cache key prefixes.
func WithCacheTenantPrefix(enabled bool) Option {
	return func(_ context.Context, s *Service) {
		s.cacheTenantPrefix = enabled
	}
}

// WithSecureProfile enables the recommended production isolation posture:
// Hybrid mode, no legacy internal Skip, TrustTenancyOnly queues, cache
// prefixes, enrollment strict, and RequireClaims binder.
//
// Residual risks (JWT tenant authenticity without ReBAC, queue tenant_id
// forgery on open brokers) are documented in the isolation design spec.
func WithSecureProfile() Option {
	return func(ctx context.Context, s *Service) {
		WithTenancySecurityMode(tenancy.ModeHybrid)(ctx, s)
		WithLegacyInternalSkip(false)(ctx, s)
		WithQueueClaimTrust(protocol.TrustTenancyOnly)(ctx, s)
		WithEnrollmentStrict(true)(ctx, s)
		WithCacheTenantPrefix(true)(ctx, s)
		// Arming auto-on for secure mode.
	}
}

// ClaimsBinder returns the service claims binder (built lazily from options).
func (s *Service) ClaimsBinder() *tenancy.ClaimsBinder {
	if s.claimsBinder != nil {
		return s.claimsBinder
	}
	secure := s.tenancySecurityMode.IsSecure() || (s.legacyInternalSkipSet && !s.legacyInternalSkip)
	b := tenancy.NewClaimsBinder(secure)
	if s.legacyInternalSkipSet {
		b.HonorInternalSkip = s.legacyInternalSkip
	}
	if s.tenancySecurityMode.IsSecure() {
		b.RequireClaims = true
		b.HonorInternalSkip = false
		if s.legacyInternalSkipSet {
			b.HonorInternalSkip = s.legacyInternalSkip
		}
	}
	s.claimsBinder = b
	return s.claimsBinder
}

func resolveTenancySecurityMode(s *Service) tenancy.SecurityMode {
	if s.tenancySecurityMode != tenancy.ModeFailOpen {
		return s.tenancySecurityMode
	}
	// Env only when option left default (zero value is FailOpen — also default).
	// If user explicitly wants FailOpen, env can still override when set.
	if v := strings.TrimSpace(os.Getenv("FRAME_TENANCY_SECURITY_MODE")); v != "" {
		if m, ok := tenancy.ParseSecurityMode(v); ok {
			return m
		}
	}
	return s.tenancySecurityMode
}

func honorInternalSkip(s *Service) bool {
	if s.legacyInternalSkipSet {
		return s.legacyInternalSkip
	}
	// Secure modes default to not honouring internal skip.
	if resolveTenancySecurityMode(s).IsSecure() {
		return false
	}
	return true
}
