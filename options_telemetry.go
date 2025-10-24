package frame

import (
	"context"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/telemetry"
)

// WithTelemetry adds required telemetry config options to the service.
func WithTelemetry(opts ...telemetry.Option) Option {
	return func(ctx context.Context, s *Service) {
		cfg, ok := s.Config().(config.ConfigurationTelemetry)
		if !ok {
			s.Log(ctx).Error("configuration object not of type : ConfigurationTelemetry")
			return
		}

		extOpts := []telemetry.Option{
			telemetry.WithServiceName(s.Name()),
			telemetry.WithServiceVersion(s.Version()),
			telemetry.WithServiceEnvironment(s.Environment())}

		extOpts = append(extOpts, opts...)

		s.telemetryManager = telemetry.NewManager(ctx, cfg, extOpts...)
		err := s.telemetryManager.Init(ctx)
		if err != nil {
			s.Log(ctx).WithError(err).Fatal("failed to initialize telemetry")
		}

		s.logger = s.telemetryManager.Log()
	}
}
