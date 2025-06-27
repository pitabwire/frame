package frame

import (
	"context"
	"log/slog"
	"runtime/debug"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/pitabwire/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// WithLogger Option that helps with initialization of our internal dbLogger.
func WithLogger() Option {
	return func(ctx context.Context, s *Service) {
		opts := util.DefaultLogOptions()

		if s.Config() != nil {
			config, ok := s.Config().(ConfigurationLogLevel)
			if ok {
				logLevelStr := config.LoggingLevel()
				logLevel, err := util.ParseLevel(logLevelStr)
				if err == nil {
					opts.Level = logLevel
				}
				opts.TimeFormat = config.LoggingTimeFormat()
				opts.NoColor = !config.LoggingColored()
				opts.ShowStackTrace = config.LoggingShowStackTrace()
				opts.PrintFormat = config.LoggingFormat()
			}
		}

		log := util.NewLogger(ctx, opts)
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

func GetLoggingOptions() []logging.Option {
	return []logging.Option{
		logging.WithLevels(func(code codes.Code) logging.Level {
			switch code {
			case codes.OK, codes.AlreadyExists:
				return logging.LevelDebug
			case codes.NotFound, codes.Canceled, codes.InvalidArgument, codes.Unauthenticated:
				return logging.LevelInfo

			case codes.DeadlineExceeded,
				codes.PermissionDenied,
				codes.ResourceExhausted,
				codes.FailedPrecondition,
				codes.Aborted,
				codes.OutOfRange,
				codes.Unavailable:
				return logging.LevelWarn

			case codes.Unknown, codes.Unimplemented, codes.Internal, codes.DataLoss:
				return logging.LevelError

			default:
				return logging.LevelError
			}
		}),
		logging.WithLogOnEvents(logging.StartCall, logging.FinishCall, logging.PayloadReceived, logging.PayloadSent),
	}
}

func RecoveryHandlerFun(ctx context.Context, p any) error {
	s := Svc(ctx)
	s.Log(ctx).WithField("trigger", p).Error("recovered from panic %s", debug.Stack())

	// Return a gRPC error
	return status.Errorf(codes.Internal, "Internal server error")
}
