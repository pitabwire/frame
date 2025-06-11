package frame

import (
	"context"
	"fmt"
	"github.com/lmittmann/tint"
	"gorm.io/gorm/logger"
	"log/slog"
	"strconv"
	"time"
)

func datbaseLogger(ctx context.Context, s *Service) logger.Interface {

	logQueries := false
	slowQueryThreshold := 200 * time.Millisecond
	if s.Config() != nil {
		config, ok := s.Config().(ConfigurationDatabase)
		if ok {
			slowQueryThreshold = config.GetDatabaseSlowQueryLogThreshold()
		}
		logQueries = config.CanDatabaseTraceQueries()
	}

	return &dbLogger{
		log:           s.Log(ctx),
		logQueries:    logQueries,
		slowThreshold: slowQueryThreshold,
	}
}

type dbLogger struct {
	log           *LogEntry
	logQueries    bool
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

	elapsed := time.Since(begin)

	sql, rows := fc()

	rowsAffected := strconv.FormatInt(rows, 10)

	log := l.log.WithContext(ctx).
		WithAttr(tint.Attr(214, slog.Any("duration", elapsed.String()))).
		WithAttr(tint.Attr(12, slog.Any("rows", rowsAffected))).
		WithAttr(tint.Attr(2, slog.Any("query", sql)))

	queryIsSlow := false
	if elapsed > l.slowThreshold && l.slowThreshold != 0 {
		log = log.WithField("SLOW Query", fmt.Sprintf(" >= %v", l.slowThreshold))
		queryIsSlow = true
	}

	if err != nil && !ErrorIsNoRows(err) {
		log.WithError(err).Error(" Error running query ")
		return
	}

	if log.LevelEnabled(ctx, slog.LevelDebug) {
		log.Debug("query executed")
		return
	}

	if log.LevelEnabled(ctx, slog.LevelInfo) {
		if l.logQueries {
			log.Info("query executed ")
		}
	}

	if log.LevelEnabled(ctx, slog.LevelWarn) {
		if queryIsSlow {
			log.Warn("query is slow")
		}
		return
	}

}
