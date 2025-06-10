package frame

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
)

const ctxKeyLogger = contextKey("loggerKey")

// LogToContext pushes a logger instance into the supplied context for easier propagation.
func LogToContext(ctx context.Context, logger *LogEntry) context.Context {
	return context.WithValue(ctx, ctxKeyLogger, logger)
}

// Log obtains a service instance being propagated through the context.
func Log(ctx context.Context) *LogEntry {
	l, ok := ctx.Value(ctxKeyLogger).(*LogEntry)
	if !ok {
		svc := Svc(ctx)
		if svc == nil {
			l = NewLogger(slog.LevelInfo).WithContext(ctx)
		} else {
			l = svc.L(ctx)
		}
	}

	return l
}

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

func (s *Service) L(ctx context.Context) *LogEntry {

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

	s := Svc(ctx)
	s.L(ctx).WithField("trigger", p).Error("recovered from panic %s", debug.Stack())

	// Return a gRPC error
	return status.Errorf(codes.Internal, "Internal server error")
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

func (l *Logger) WithContext(ctx context.Context) *LogEntry {
	l.ctx = ctx
	return &LogEntry{l: l}
}

func (l *Logger) WithError(err error) *LogEntry {
	l.slog = l.slog.With("error", err)
	return &LogEntry{l: l}
}

func (l *Logger) WithField(key string, value any) *LogEntry {
	l.slog = l.slog.With(key, value)
	return &LogEntry{l: l}
}

func (l *Logger) WithFields(key string, value any) *LogEntry {
	l.slog = l.slog.With(key, value)
	return &LogEntry{l: l}
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

// LogEntry Need a type to handle the chained calls
type LogEntry struct {
	l *Logger
}

type exitFunc func(int)

func (e *LogEntry) Level() slog.Level {
	return e.l.level
}

func (e *LogEntry) WithContext(ctx context.Context) *LogEntry {
	e.l.WithContext(ctx)
	return e
}

func (e *LogEntry) Log(ctx context.Context, level slog.Level, msg string, fields ...any) {
	e.l.Log(ctx, level, msg, fields...)
}

func (e *LogEntry) Debug(msg string, args ...any) {
	e.l.Debug(msg, args...)
}

func (e *LogEntry) Info(msg string, args ...any) {
	e.l.Info(msg, args...)
}

func (e *LogEntry) Printf(format string, args ...any) {
	e.Info(format, args...)
}

func (e *LogEntry) Warn(msg string, args ...any) {
	e.l.Warn(msg, args...)
}

func (e *LogEntry) Error(msg string, args ...any) {
	e.l.Error(msg, args...)
}

func (e *LogEntry) Fatal(msg string, args ...any) {
	e.l.Fatal(msg, args...)

}

func (e *LogEntry) Panic(msg string, args ...any) {
	e.l.Panic(msg, args...)
}

func (e *LogEntry) WithError(err error) *LogEntry {
	e.l.slog = e.l.slog.With("error", err)
	return e
}
func (e *LogEntry) WithField(key string, value any) *LogEntry {
	e.l.slog = e.l.slog.With(key, value)
	return e
}
