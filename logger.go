package frame

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
	"log/slog"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// WithLogger Option that helps with initialization of our internal dbLogger
func WithLogger() Option {
	return func(s *Service) {

		logLevelStr := "info"

		if s.Config() != nil {
			config, ok := s.Config().(ConfigurationLogLevel)
			if ok {
				logLevelStr = config.LoggingLevel()
			}
		}

		logLevel, err := ParseLevel(logLevelStr)
		if err != nil {
			logLevel = slog.LevelInfo
		}

		s.logger = NewLogger(logLevel)
	}
}

func (s *Service) L(ctx context.Context) *Entry {

	return s.logger.WithContext(ctx).WithField("service", s.Name())
}

func (s *Service) SLog(_ context.Context) *slog.Logger {
	return s.logger.slog.With("service", s.Name())
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

func RecoveryHandlerFun(ctx context.Context, p interface{}) error {

	s := FromContext(ctx)
	s.L(ctx).WithField("trigger", p).Error("recovered from panic %s", debug.Stack())

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

	l.s.L(ctx).Info(msg, data...)
}

// Warn print warn messages
func (l *dbLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	l.s.L(ctx).Warn(msg, data...)
}

// Error print error messages
func (l *dbLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	l.s.L(ctx).Error(msg, data...)
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
	case log.Level() >= slog.LevelWarn && slowQuery:
		log.Warn("SLOW Query ")
	case log.Level() >= slog.LevelInfo && slowQuery:
		log.Info("SLOW Query ")
	case log.Level() >= slog.LevelDebug:
		log.Info("Query Debug ")

	}
}

type Logger struct {
	ctx context.Context
	// Function to exit the application, defaults to `os.Exit()`
	ExitFunc exitFunc
	level    slog.Level
	slog     *slog.Logger
}

// ParseLevel converts a string to a slog.Level.
// It is case-insensitive.
// Returns an error if the string does not match a known level.
func ParseLevel(levelStr string) (slog.Level, error) {
	switch strings.ToLower(levelStr) {
	case "debug", "trace":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning": // Add "warning" as an alias for "warn" if desired
		return slog.LevelWarn, nil
	case "error", "fatal", "panic":
		return slog.LevelError, nil
	default:
		// Default to Info or return an error for unrecognized strings
		return slog.LevelInfo, fmt.Errorf("unknown log level: %q", levelStr)
	}
}

func NewLogger(logLevel slog.Level) *Logger {

	outputWriter := os.Stdout
	if logLevel >= slog.LevelError {
		outputWriter = os.Stderr
	}

	handlerOptions := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewTextHandler(outputWriter, handlerOptions)
	newLogger := slog.New(handler)

	return &Logger{slog: newLogger, level: logLevel}
}

func (l *Logger) WithContext(ctx context.Context) *Entry {
	l.ctx = ctx
	return &Entry{l: l}
}

func (l *Logger) WithError(err error) *Entry {
	l.slog = l.slog.With("error", err)
	return &Entry{l: l}
}

func (l *Logger) WithField(key string, value any) *Entry {
	l.slog = l.slog.With(key, value)
	return &Entry{l: l}
}

func (l *Logger) WithFields(key string, value any) *Entry {
	l.slog = l.slog.With(key, value)
	return &Entry{l: l}
}

func (l *Logger) _ctx() context.Context {
	if l.ctx == nil {
		return context.Background()
	}
	return l.ctx
}

func (l *Logger) Log(ctx context.Context, level slog.Level, msg string, fields ...any) {
	l.slog.Log(ctx, level, msg, fields...)
}

func (l *Logger) Debug(msg string, args ...any) {
	l.slog.DebugContext(l._ctx(), msg, args...)
}

func (l *Logger) Info(msg string, args ...any) {
	l.slog.InfoContext(l._ctx(), msg, args...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.slog.WarnContext(l._ctx(), msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {
	l.slog.ErrorContext(l._ctx(), msg, args...)
}

func (l *Logger) Fatal(msg string, args ...any) {
	l.slog.ErrorContext(l._ctx(), msg, args...)
	l.Exit(1)
}

func (l *Logger) Panic(msg string, args ...any) {
	l.slog.ErrorContext(l._ctx(), msg, args...)
	panic(msg)
}

func (l *Logger) Exit(code int) {
	if l.ExitFunc == nil {
		l.ExitFunc = os.Exit
	}
	l.ExitFunc(code)
}

// Entry Need a type to handle the chained calls
type Entry struct {
	l *Logger
}

type exitFunc func(int)

func (e *Entry) Level() slog.Level {
	return e.l.level
}

func (e *Entry) WithContext(ctx context.Context) *Entry {
	e.l.WithContext(ctx)
	return e
}

func (e *Entry) Log(ctx context.Context, level slog.Level, msg string, fields ...any) {
	e.l.Log(ctx, level, msg, fields...)
}

func (e *Entry) Debug(msg string, args ...any) {
	e.l.Debug(msg, args...)
}

func (e *Entry) Info(msg string, args ...any) {
	e.l.Info(msg, args...)
}

func (e *Entry) Printf(format string, args ...any) {
	e.Info(format, args...)
}

func (e *Entry) Warn(msg string, args ...any) {
	e.l.Warn(msg, args...)
}

func (e *Entry) Error(msg string, args ...any) {
	e.l.Error(msg, args...)
}

func (e *Entry) Fatal(msg string, args ...any) {
	e.l.Fatal(msg, args...)

}

func (e *Entry) Panic(msg string, args ...any) {
	e.l.Panic(msg, args...)
}

func (e *Entry) WithError(err error) *Entry {
	e.l.slog = e.l.slog.With("error", err)
	return e
}
func (e *Entry) WithField(key string, value any) *Entry {
	e.l.slog = e.l.slog.With(key, value)
	return e
}
