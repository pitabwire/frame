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
		if s.Config() != nil {
			config, ok := s.Config().(config2.ConfigurationLogLevel)
			if ok {
				logLevelStr := config.LoggingLevel()
				logLevel, err := util.ParseLevel(logLevelStr)
				if err == nil {
					opts = append(opts, util.WithLogLevel(logLevel))
				}
				opts = append(opts,
					util.WithLogTimeFormat(config.LoggingTimeFormat()),
					util.WithLogNoColor(!config.LoggingColored()),
					util.WithLogStackTrace())
			}
		}

		// Add the telemetry log handler to the logger
		if s.telemetryManager != nil {
			opts = append(opts, util.WithLogHandler(s.telemetryManager.LogHandler()))
		}

		log := util.NewLogger(ctx, opts...)
		log.WithField("service", s.Name())
		s.logger = log
	}
}

func (s *Service) Log(ctx context.Context) *util.LogEntry {
	return s.logger.WithContext(ctx)
}

func (s *Service) SLog(ctx context.Context) *slog.Logger {
	return s.Log(ctx).SLog()
}
