package frame

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"os"
	"runtime/debug"
)

// Logger Option that helps with initialization of our internal logger
func Logger() Option {
	return func(s *Service) {

		logLevelStr := "info"

		if s.Config() != nil {
			config, ok := s.Config().(ConfigurationLogLevel)
			if ok {
				logLevelStr = config.LoggingLevel()
			}
		}

		logLevel, err := logrus.ParseLevel(logLevelStr)
		if err != nil {
			logLevel = logrus.InfoLevel
		}

		s.logger = logrus.New()
		// set global log level
		s.logger.SetLevel(logLevel)

		s.logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
			DisableQuote:  true,
		})
		s.logger.SetReportCaller(true)
		s.logger.SetOutput(io.Discard)
		s.logger.AddHook(&writer.Hook{
			Writer: os.Stderr,
			LogLevels: []logrus.Level{
				logrus.PanicLevel,
				logrus.FatalLevel,
				logrus.ErrorLevel,
				logrus.WarnLevel,
			},
		})
		s.logger.AddHook(&writer.Hook{
			Writer: os.Stdout,
			LogLevels: []logrus.Level{
				logrus.InfoLevel,
				logrus.DebugLevel,
				logrus.TraceLevel,
			},
		})

	}
}

func (s *Service) L(ctx context.Context) *logrus.Entry {
	return s.logger.WithContext(ctx).WithField("service", s.Name())
}

func GetLoggingOptions() []logging.Option {
	return []logging.Option{
		logging.WithLevels(func(code codes.Code) logging.Level {
			switch code {
			case codes.OK, codes.AlreadyExists:
				return logging.LevelDebug
			case codes.NotFound, codes.Canceled, codes.InvalidArgument, codes.Unauthenticated:
				return logging.LevelInfo

			case codes.DeadlineExceeded, codes.PermissionDenied, codes.ResourceExhausted, codes.FailedPrecondition, codes.Aborted,
				codes.OutOfRange, codes.Unavailable:
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

// LoggingInterceptor adapts logrus logger to interceptor logger.
func LoggingInterceptor(l logrus.FieldLogger) logging.Logger {
	return logging.LoggerFunc(func(_ context.Context, lvl logging.Level, msg string, fields ...any) {
		f := make(map[string]any, len(fields)/2)
		i := logging.Fields(fields).Iterator()
		if i.Next() {
			k, v := i.At()
			f[k] = v
		}
		l := l.WithFields(f)

		switch lvl {
		case logging.LevelDebug:
			l.Debug(msg)
		case logging.LevelInfo:
			l.Info(msg)
		case logging.LevelWarn:
			l.Warn(msg)
		case logging.LevelError:
			l.Error(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}

func RecoveryHandlerFun(ctx context.Context, p interface{}) error {

	s := FromContext(ctx)
	logger := s.L(ctx)
	logger.WithField("trigger", p).Errorf("recovered from panic %s", debug.Stack())

	// Return a gRPC error
	return status.Errorf(codes.Internal, "Internal server error")
}
