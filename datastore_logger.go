package frame

import (
	"context"
	"fmt"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
	"log/slog"
	"strconv"
	"time"
)

func buildDBLogger(ctx context.Context, s *Service) logger.Interface {

	slowQueryThreshold := 200 * time.Millisecond
	logQueries := false
	if s.Config() != nil {
		config, ok := s.Config().(ConfigurationDatabase)
		if ok {
			slowQueryThreshold = config.GetSlowQueryThreshold()
		}
		logQueries = config.CanLogDatabaseQueries()
	}

	return &dbLogger{
		log:           s.L(ctx),
		canLogQueries: logQueries,
		slowThreshold: slowQueryThreshold,
	}

}

type dbLogger struct {
	log *LogEntry

	canLogQueries bool
	slowThreshold time.Duration
}

// LogMode log mode
func (l *dbLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l
}

// Info print info
func (l *dbLogger) Info(ctx context.Context, msg string, data ...interface{}) {

	l.log.WithContext(ctx).Info(msg, data...)
}

// Warn print warn messages
func (l *dbLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	l.log.WithContext(ctx).Warn(msg, data...)
}

// Error print error messages
func (l *dbLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	l.log.WithContext(ctx).Error(msg, data...)
}

// Trace print sql message
func (l *dbLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {

	if !l.canLogQueries {
		return
	}

	elapsed := time.Since(begin)

	sql, rows := fc()

	rowsAffected := strconv.FormatInt(rows, 10)

	log := l.log.WithContext(ctx).WithField("query", sql).WithField("duration", elapsed.String()).WithField("rows", rowsAffected).WithField("file", utils.FileWithLineNum())

	queryIsSlow := false
	if elapsed > l.slowThreshold && l.slowThreshold != 0 {
		log = log.WithField("SLOW Query", fmt.Sprintf(" >= %v", l.slowThreshold))
		queryIsSlow = true
	}

	switch {
	case err != nil && !ErrorIsNoRows(err):
		log.WithError(err).Error("Query Error")
	case log.Level() >= slog.LevelWarn && queryIsSlow:
		log.Warn("SLOW Query ")
	case log.Level() >= slog.LevelInfo && queryIsSlow:
		log.Info("SLOW Query ")
	case log.Level() == slog.LevelDebug:
		log.Debug("Query Debug ")

	}
}
