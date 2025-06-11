package frame

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lmittmann/tint"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

const ctxKeyLogger = contextKey("loggerKey")

// LogToContext pushes a logger instance into the supplied context for easier propagation.
func LogToContext(ctx context.Context, logger *LogEntry) context.Context {
	return context.WithValue(ctx, ctxKeyLogger, logger)
}

// Log obtains a service instance being propagated through the context.
func Log(ctx context.Context) *LogEntry {
	logEntry, ok := ctx.Value(ctxKeyLogger).(*LogEntry)
	if !ok {
		svc := Svc(ctx)
		if svc == nil {
			log := NewLogger(defaultLogOptions())
			log.ctx = ctx
			logEntry = &LogEntry{l: log}
		} else {
			logEntry = svc.Log(ctx)
		}
	}

	return logEntry
}

// WithLogger Option that helps with initialization of our internal dbLogger
func WithLogger() Option {
	return func(s *Service) {

		opts := defaultLogOptions()

		if s.Config() != nil {
			config, ok := s.Config().(ConfigurationLogLevel)
			if ok {
				logLevelStr := config.LoggingLevel()
				logLevel, err := ParseLevel(logLevelStr)
				if err == nil {
					opts.Level = logLevel
				}
				opts.TimeFormat = config.LoggingTimeFormat()
				opts.NoColor = !config.LoggingColored()
				opts.PrintFormat = config.LoggingFormat()
			}
		}

		log := NewLogger(opts)
		log.WithFields("service", s.Name())
		s.logger = log
	}
}

func (s *Service) Log(ctx context.Context) *LogEntry {
	return &LogEntry{
		l: s.logger.clone(ctx),
	}
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
	s.Log(ctx).WithField("trigger", p).Error("recovered from panic %s", debug.Stack())

	// Return a gRPC error
	return status.Errorf(codes.Internal, "Internal server error")
}

type Logger struct {
	ctx context.Context
	// Function to exit the application, defaults to `os.Exit()`
	ExitFunc exitFunc
	slog     *slog.Logger
}

type LogOptions struct {
	*slog.HandlerOptions
	PrintFormat string
	TimeFormat  string
	NoColor     bool
}

func defaultLogOptions() *LogOptions {
	return &LogOptions{
		HandlerOptions: &slog.HandlerOptions{
			AddSource: false,
			Level:     slog.LevelInfo,
		},
		TimeFormat: time.DateTime,
		NoColor:    false,
	}
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

func NewLogger(options *LogOptions) *Logger {

	logLevel := options.Level.Level()
	outputWriter := os.Stdout
	if logLevel >= slog.LevelError {
		outputWriter = os.Stderr
	}

	handlerOptions := &tint.Options{
		AddSource:  options.AddSource,
		Level:      logLevel,
		TimeFormat: options.TimeFormat,
		NoColor:    options.NoColor,
	}

	handler := tint.NewHandler(outputWriter, handlerOptions)

	newLogger := slog.New(handler)

	slog.SetDefault(newLogger)

	return &Logger{slog: newLogger}
}

func (l *Logger) clone(ctx context.Context) *Logger {
	sl := *l.slog
	return &Logger{ctx: ctx, slog: &sl}
}

func (l *Logger) WithError(err error) {
	l.slog = l.slog.With(tint.Err(err))
}

func (l *Logger) WithAttr(attr ...any) {
	l.slog = l.slog.With(attr...)
}

func (l *Logger) WithField(key string, value any) {
	l.slog = l.slog.With(key, value)
}

func (l *Logger) WithFields(key string, value any) {
	l.slog = l.slog.With(key, value)
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
	var log *slog.Logger
	fileLineNum := l.fileWithLineNum()
	if fileLineNum != "" {
		log = l.slog.With(tint.Attr(4, slog.Any("file", fileLineNum)))
	} else {
		log = l.slog
	}
	log.DebugContext(l._ctx(), msg, args...)
}

func (l *Logger) Info(msg string, args ...any) {
	l.slog.InfoContext(l._ctx(), msg, args...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.slog.WarnContext(l._ctx(), msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {

	var log *slog.Logger
	fileLineNum := l.fileWithLineNum()
	if fileLineNum != "" {
		log = l.slog.With(tint.Attr(4, slog.Any("file", fileLineNum)))
	} else {
		log = l.slog
	}

	log.ErrorContext(l._ctx(), msg, args...)
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

func (l *Logger) Enabled(ctx context.Context, level slog.Level) bool {
	return l.slog.Enabled(ctx, level)
}

func (l *Logger) fileWithLineNum() string {
	_, file, line, ok := runtime.Caller(3)
	if ok {
		return fmt.Sprintf("%s:%d", file, line)
	}
	return ""
}

// LogEntry Need a type to handle the chained calls
type LogEntry struct {
	l *Logger
}

type exitFunc func(int)

func (e *LogEntry) LevelEnabled(ctx context.Context, level slog.Level) bool {
	return e.l.Enabled(ctx, level)
}

func (e *LogEntry) WithContext(ctx context.Context) *LogEntry {
	return &LogEntry{e.l.clone(ctx)}
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

func (e *LogEntry) WithAttr(attr ...any) *LogEntry {
	e.l.WithAttr(attr...)
	return e
}

func (e *LogEntry) WithError(err error) *LogEntry {
	e.l.WithError(err)
	return e
}
func (e *LogEntry) WithField(key string, value any) *LogEntry {
	e.l.WithField(key, value)
	return e
}
