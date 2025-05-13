package frame

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
	"io"
	"os"
	"runtime/debug"
	"strconv"
	"time"
)

// Logger Option that helps with initialization of our internal dbLogger
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

// LoggingInterceptor adapts logrus dbLogger to interceptor dbLogger.
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

func buildDBLogger(s *Service) logger.Interface {

	slowQueryThreshold := 200 * time.Millisecond

	if s.Config() != nil {
		config, ok := s.Config().(ConfigurationDatabase)
		if ok {
			slowQueryThreshold = config.GetSlowQueryThreshold()
		}
	}

	return &dbLogger{
		s:             s,
		SlowThreshold: slowQueryThreshold,
	}

}

type dbLogger struct {
	s *Service

	SlowThreshold time.Duration
}

// LogMode log mode
func (l *dbLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l
}

// Info print info
func (l *dbLogger) Info(ctx context.Context, msg string, data ...interface{}) {

	args := append([]any{msg}, data...)
	l.s.L(ctx).Info(args...)
}

// Warn print warn messages
func (l *dbLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	args := append([]any{msg}, data...)
	l.s.L(ctx).Warn(args...)
}

// Error print error messages
func (l *dbLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	args := append([]any{msg}, data...)
	l.s.L(ctx).Error(args...)
}

// Trace print sql message
func (l *dbLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {

	elapsed := time.Since(begin)

	sql, rows := fc()

	rowsAffected := "-"
	if rows != -1 {
		rowsAffected = strconv.FormatInt(rows, 10)
	}

	log := l.s.L(ctx).WithField("query", sql).WithField("duration", float64(elapsed.Nanoseconds())/1e6).WithField("rows", rowsAffected).WithField("file", utils.FileWithLineNum())

	slowQuery := false
	if elapsed > l.SlowThreshold && l.SlowThreshold != 0 {
		log = log.WithField("SLOW Query", fmt.Sprintf(" >= %v", l.SlowThreshold))
		slowQuery = true
	}

	switch {
	case err != nil && !ErrorIsNoRows(err):
		log.WithError(err).Error(" Query Error : ")
	case log.Level >= logrus.WarnLevel && slowQuery:
		log.Warn("SLOW Query ")
	case log.Level >= logrus.InfoLevel && slowQuery:
		log.Info("SLOW Query ")
	case log.Level >= logrus.DebugLevel:
		log.Info("Query Debug ")

	}
}
