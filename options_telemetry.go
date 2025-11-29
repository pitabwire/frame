package frame

import (
	"context"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/telemetry"
)

// WithTelemetry adds required telemetry config options to the service.
func WithTelemetry(opts ...telemetry.Option) Option {
	return func(ctx context.Context, s *Service) {
		cfg, ok := s.Config().(config.ConfigurationTelemetry)
		if !ok {
			util.Log(ctx).Error("configuration object not of type : ConfigurationTelemetry")
			return
		}

		extOpts := []telemetry.Option{
			telemetry.WithServiceName(s.Name()),
			telemetry.WithServiceVersion(s.Version()),
			telemetry.WithServiceEnvironment(s.Environment())}

		if cfg.DisableOpenTelemetry() {
			extOpts = append(extOpts, telemetry.WithDisableTracing())
		}

		extOpts = append(extOpts, opts...)

		s.telemetryManager = telemetry.NewManager(ctx, cfg, extOpts...)
		err := s.telemetryManager.Init(ctx)
		if err != nil {
			util.Log(ctx).WithError(err).Fatal("failed to initialize telemetry")
		}
	}
}

func (s *Service) TelemetryManager() telemetry.Manager {
	return s.telemetryManager
}
