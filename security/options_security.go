package security

import (
	"context"

	"github.com/pitabwire/frame/client"
	"github.com/pitabwire/frame/config"
)

// Options contains configuration for security manager.
type Options struct {
	SecurityCfg config.ConfigurationSecurity
	Oath2Cfg    config.ConfigurationOAUTH2
	ServiceCfg  config.ConfigurationService
	Invoker     client.HTTPInvoker
}

type Option func(ctx context.Context, opts *Options)

// WithSecurityConfig adds a security configuration to existing Options.
func WithSecurityConfig(cfg config.ConfigurationSecurity) Option {
	return func(_ context.Context, opts *Options) {
		opts.SecurityCfg = cfg
	}
}

// WithOauth2Config adds an oauth2 configuration to Options.
func WithOauth2Config(cfg config.ConfigurationOAUTH2) Option {
	return func(_ context.Context, opts *Options) {
		opts.Oath2Cfg = cfg
	}
}

// WithServiceConfig adds service configuration to Options.
func WithServiceConfig(cfg config.ConfigurationService) Option {
	return func(_ context.Context, opts *Options) {
		opts.ServiceCfg = cfg
	}
}

// WithInvoker adds an oauth2 configuration to Options.
func WithInvoker(cfg client.HTTPInvoker) Option {
	return func(_ context.Context, opts *Options) {
		opts.Invoker = cfg
	}
}
