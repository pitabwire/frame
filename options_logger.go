package frame

import (
	"context"
	"log/slog"

	"github.com/pitabwire/util"

	config2 "github.com/pitabwire/frame/config"
)

// WithLogger Option that helps with initialization of our internal dbLogger.
func WithLogger(opts ...util.Option) Option {
	return func(ctx context.Context, s *Service) {
		var configOpts []util.Option

		// Add telemetry log handler if available
		if s.telemetryManager != nil {
			configOpts = append(configOpts, util.WithLogHandler(s.telemetryManager.LogHandler()))
		}

		// Early return if no config is available
		if s.Config() == nil {
			log := util.NewLogger(ctx, append(configOpts, opts...)...)
			s.logger = log.WithField("service", s.Name())
			return
		}

		config, ok := s.Config().(config2.ConfigurationLogLevel)
		if !ok {
			log := util.NewLogger(ctx, append(configOpts, opts...)...)
			s.logger = log.WithField("service", s.Name())
			return
		}

		// Parse log level
		if logLevel, err := util.ParseLevel(config.LoggingLevel()); err == nil {
			configOpts = append(configOpts, util.WithLogLevel(logLevel))
		}

		// Add standard config options
		configOpts = append(configOpts,
			util.WithLogTimeFormat(config.LoggingTimeFormat()),
			util.WithLogNoColor(!config.LoggingColored()))

		// Add stack trace if enabled
		if config.LoggingShowStackTrace() {
			configOpts = append(configOpts, util.WithLogStackTrace())
		}

		log := util.NewLogger(ctx, append(configOpts, opts...)...)
		s.logger = log.WithField("service", s.Name())
	}
}

func (s *Service) Log(ctx context.Context) *util.LogEntry {
	return s.logger.WithContext(ctx)
}

func (s *Service) SLog(ctx context.Context) *slog.Logger {
	return s.Log(ctx).SLog()
}
