package frame

import "go.uber.org/zap"

type ILogger interface {
	Debug(template string, args ...interface{})
	Info(template string, args ...interface{})
	Warn(template string, args ...interface{})
	Error(template string, args ...interface{})
	Panic(template string, args ...interface{})
	Fatal(template string, args ...interface{})
}

type internalLogger struct {
	l *zap.SugaredLogger
}

// Debug uses fmt.Sprint to construct and log a message.
func (iLogger *internalLogger) Debug(template string, args ...interface{}) {
	iLogger.l.Debugf(template, args...)
}

// Info uses fmt.Sprint to construct and log a message.
func (iLogger *internalLogger) Info(template string, args ...interface{}) {
	iLogger.l.Infof(template, args...)
}

// Warn uses fmt.Sprint to construct and log a message.
func (iLogger *internalLogger) Warn(template string, args ...interface{}) {
	iLogger.l.Warnf(template, args...)
}

// Error uses fmt.Sprint to construct and log a message.
func (iLogger *internalLogger) Error(template string, args ...interface{}) {
	iLogger.l.Errorf(template, args...)
}

// Panic uses fmt.Sprint to construct and log a message, then panics.
func (iLogger *internalLogger) Panic(template string, args ...interface{}) {
	iLogger.l.Panicf(template, args...)
}

// Fatal uses fmt.Sprint to construct and log a message, then calls os.Exit.
func (iLogger *internalLogger) Fatal(template string, args ...interface{}) {
	iLogger.l.Fatalf(template, args...)
}

// Logger Option that helps with initialization of our internal logger
func Logger() Option {
	return func(s *Service) {

		l, _ := zap.NewProduction()
		sugaredL := l.Sugar()
		s.logger = &internalLogger{l: sugaredL}

		s.AddCleanupMethod(func() {
			_ = sugaredL.Sync()
		})

	}
}

func (s *Service) L() ILogger {
	return s.logger
}
