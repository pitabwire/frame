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

	if s.Config() != nil {
		config, ok := s.Config().(ConfigurationDatabase)
		if ok {
			slowQueryThreshold = config.GetSlowQueryThreshold()
		}
	}

	return &dbLogger{
		log:           s.L(ctx),
		SlowThreshold: slowQueryThreshold,
	}

}

type dbLogger struct {
	log *LogEntry

	SlowThreshold time.Duration
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

	elapsed := time.Since(begin)

	sql, rows := fc()

	rowsAffected := "-"
	if rows != -1 {
		rowsAffected = strconv.FormatInt(rows, 10)
	}

	log := l.log.WithContext(ctx).WithField("query", sql).WithField("duration", float64(elapsed.Nanoseconds())/1e6).WithField("rows", rowsAffected).WithField("file", utils.FileWithLineNum())

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
	case log.Level() == slog.LevelDebug:
		log.Debug("Query Debug ")

	}
}
