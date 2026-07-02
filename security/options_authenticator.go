package security

import (
	"context"

	"github.com/pitabwire/frame/v2/config"
)

// AuthOptions contains configuration for Redis cache.
type AuthOptions struct {
	DisableSecurityCfg config.ConfigurationSecurity
	DisableSecurity    bool
}

type AuthOption func(ctx context.Context, opts *AuthOptions)

// WithDisableSecurityConfig adds a security configuration to existing AuthOptions.
func WithDisableSecurityConfig(cfg config.ConfigurationSecurity) AuthOption {
	return func(_ context.Context, opts *AuthOptions) {
		opts.DisableSecurityCfg = cfg
	}
}

// WithDisableSecurity sets the security should be disabled.
func WithDisableSecurity() AuthOption {
	return func(_ context.Context, opts *AuthOptions) {
		opts.DisableSecurity = true
	}
}
