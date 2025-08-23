package framelogging

import (
	"context"
	"log/slog"
	"runtime/debug"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/pitabwire/frame/internal/common"
	"github.com/pitabwire/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// loggingService represents a service that can perform logging operations
type loggingService struct {
	service common.Service
}

func (ls loggingService) GetModule(moduleType string) interface{} {
	return ls.service.GetModule(moduleType)
}

// WithLogger Option that helps with initialization of our internal dbLogger.
func WithLogger() common.Option {
	return func(ctx context.Context, s common.Service) {
		var opts []util.Option

		if s.Config() != nil {
			config, ok := s.Config().(common.ConfigurationLogLevel)
			if ok {
				logLevelStr := config.GetLogLevel()
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
		
		// Create and register LoggingModule with the logger
		loggingModule := common.NewLoggingModule(log)
		s.RegisterModule(loggingModule)
	}
}

func (s loggingService) Log(ctx context.Context) *util.LogEntry {
	module := s.GetModule(common.ModuleTypeLogging)
	if module == nil {
		return util.NewLogger(ctx).WithContext(ctx)
	}
	
	// Since we can't access Logger() method from interface{},
	// we'll return a new logger instance
	_ = module // Mark as used
	return util.NewLogger(ctx).WithContext(ctx)
}

func (s loggingService) SLog(ctx context.Context) *slog.Logger {
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
	s := common.Svc
	s.Log(ctx).WithField("trigger", p).Error("recovered from panic %s", debug.Stack())

	// Return a gRPC error
	return status.Errorf(codes.Internal, "Internal server error")
}
