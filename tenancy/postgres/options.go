package postgres

import "github.com/pitabwire/frame/v2/tenancy"

// Option configures a Provider.
type Option func(*Provider)

// WithSecurityMode sets the provider security mode (default ModeFailOpen).
func WithSecurityMode(m tenancy.SecurityMode) Option {
	return func(p *Provider) {
		p.mode = m
	}
}

// WithAllowGlobalServices sets the allowlist of ServiceName values that may
// use SystemPrincipal.AllowGlobal for match-all elevation.
func WithAllowGlobalServices(names ...string) Option {
	return func(p *Provider) {
		p.setAllowGlobalServices(names...)
	}
}

// WithEnrollmentStrict enables strict enrollment: migrate fails if models
// look tenanted but do not implement tenancy.Tenanted and are not Unscoped.
func WithEnrollmentStrict(strict bool) Option {
	return func(p *Provider) {
		p.enrollmentStrict = strict
	}
}

// SetEnrollmentStrict updates strict enrollment after construction.
func (p *Provider) SetEnrollmentStrict(strict bool) {
	p.enrollmentStrict = strict
}
