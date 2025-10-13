package frame

import (
	"context"
	"log/slog"

	"github.com/pitabwire/util"
)

// WithLogger Option that helps with initialization of our internal dbLogger.
func WithLogger(opts ...util.Option) Option {
	return func(ctx context.Context, s *Service) {
		if s.Config() != nil {
			config, ok := s.Config().(ConfigurationLogLevel)
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
