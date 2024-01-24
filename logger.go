package frame

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
	"io"
	"os"
	"strings"
)

// Logger Option that helps with initialization of our internal logger
func Logger() Option {
	return func(s *Service) {

		logLevel := "info"

		if s.Config() != nil {
			oauth2Config, ok := s.Config().(ConfigurationDefault)
			if ok {
				logLevel = oauth2Config.LogLevel
			}
		}

		logger := logrus.New()
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
			DisableQuote:  true,
		})
		logger.SetReportCaller(true)
		logger.SetOutput(io.Discard)
		logger.AddHook(&writer.Hook{
			Writer: os.Stderr,
			LogLevels: []logrus.Level{
				logrus.PanicLevel,
				logrus.FatalLevel,
				logrus.ErrorLevel,
				logrus.WarnLevel,
			},
		})
		logger.AddHook(&writer.Hook{
			Writer: os.Stdout,
			LogLevels: []logrus.Level{
				logrus.InfoLevel,
				logrus.DebugLevel,
			},
		})

		logLevel = strings.ToLower(logLevel)
		switch logLevel {
		case "debug":
			logger.SetLevel(logrus.DebugLevel)
			break
		case "trace":
			logger.SetLevel(logrus.TraceLevel)
			break
		case "warn":
			logger.SetLevel(logrus.WarnLevel)
			break
		case "error":
			logger.SetLevel(logrus.ErrorLevel)
			break
		case "fatal":
			logger.SetLevel(logrus.FatalLevel)
			break
		default:

			logger.SetLevel(logrus.InfoLevel)
			break
		}

		s.logger = logger
	}
}

func (s *Service) L() *logrus.Entry {
	return s.logger.WithField("service", s.Name())
}

func GetLoggingOptions() []logging.Option {
	return []logging.Option{
		logging.WithLogOnEvents(logging.StartCall, logging.FinishCall),
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
