package security

import (
	"context"

	"github.com/pitabwire/frame/config"
)

// AuthOptions contains configuration for Redis cache.
type AuthOptions struct {
	DisableSecurityCfg config.ConfigurationSecurity
	Audience           []string
	Issuer             string
	DisableSecurity    bool
}

type AuthOption func(ctx context.Context, opts *AuthOptions)

// WithDisableSecurityConfig adds a security configuration to existing AuthOptions.
func WithDisableSecurityConfig(cfg config.ConfigurationSecurity) AuthOption {
	return func(_ context.Context, opts *AuthOptions) {
		opts.DisableSecurityCfg = cfg
	}
}

// WithAudience sets the audience to use overriding any config option.
func WithAudience(audience ...string) AuthOption {
	return func(_ context.Context, opts *AuthOptions) {
		opts.Audience = audience
	}
}

// WithIssuer sets the issuer to use overriding any config option.
func WithIssuer(issuer string) AuthOption {
	return func(_ context.Context, opts *AuthOptions) {
		opts.Issuer = issuer
	}
}

// WithDisableSecurity sets the security should be disabled.
func WithDisableSecurity() AuthOption {
	return func(_ context.Context, opts *AuthOptions) {
		opts.DisableSecurity = true
	}
}
