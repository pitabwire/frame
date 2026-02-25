package frame

import (
	"context"

	"github.com/pitabwire/frame/config"
)

// WithConfig Option that helps to specify or override the configuration object of our service.
func WithConfig(cfg any) Option {
	return func(ctx context.Context, s *Service) {
		s.configuration = cfg

		serviceCfg, ok := cfg.(config.ConfigurationService)
		if ok {
			if serviceCfg.Name() != "" {
				WithName(serviceCfg.Name())(ctx, s)
			}

			if serviceCfg.Environment() != "" {
				WithEnvironment(serviceCfg.Environment())(ctx, s)
			}

			if serviceCfg.Version() != "" {
				WithVersion(serviceCfg.Version())(ctx, s)
			}
		}

		WithTelemetry()(ctx, s)

		WithLogger()(ctx, s)

		WithHTTPClient()(ctx, s)

		if dbgCfg, ok := cfg.(config.ConfigurationDebug); ok {
			if dbgCfg.DebugEndpointsEnabled() {
				WithDebugEndpointsAt(dbgCfg.DebugEndpointsBasePath())(ctx, s)
			}
		}
	}
}

func (s *Service) Config() any {
	return s.configuration
}
