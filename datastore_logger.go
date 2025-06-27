package frame

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/lmittmann/tint"
	"github.com/pitabwire/util"
	glogger "gorm.io/gorm/logger"
)

const (
	tintAttrCodeDuration = 214
	tintAttrCodeRows     = 12
	tintAttrCodeQuery    = 2

	DefaultSlowQueryThreshold = 200 * time.Millisecond
)

func datbaseLogger(ctx context.Context, s *Service) glogger.Interface {
	logQueries := false
	slowQueryThreshold := DefaultSlowQueryThreshold
	if s.Config() != nil {
		config, ok := s.Config().(ConfigurationDatabase)
		if ok {
			slowQueryThreshold = config.GetDatabaseSlowQueryLogThreshold()
		}
		logQueries = config.CanDatabaseTraceQueries()
	}

	return &dbLogger{
		logQueries:    logQueries,
		slowThreshold: slowQueryThreshold,
		baseLogger:    util.NewLogger(ctx, util.DefaultLogOptions()),
	}
}

type dbLogger struct {
	baseLogger    *util.LogEntry // Base logger to clone for each query to avoid attribute accumulation
	logQueries    bool
	slowThreshold time.Duration
}

// LogMode log mode.
func (l *dbLogger) LogMode(_ glogger.LogLevel) glogger.Interface {
	return l
}

// Info print info.
func (l *dbLogger) Info(ctx context.Context, msg string, data ...any) {
	log := l.baseLogger.WithContext(ctx)
	log.Info(msg, data...)
}

// Warn print warn messages.
func (l *dbLogger) Warn(ctx context.Context, msg string, data ...any) {
	log := l.baseLogger.WithContext(ctx)
	log.Warn(msg, data...)
}

// Error print error messages.
func (l *dbLogger) Error(ctx context.Context, msg string, data ...any) {
	log := l.baseLogger.WithContext(ctx)
	log.Error(msg, data...)
}

// Trace print sql message.
func (l *dbLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)

	sql, rows := fc()

	rowsAffected := strconv.FormatInt(rows, 10)

	log := l.baseLogger.WithContext(ctx).
		With(
			tint.Attr(tintAttrCodeDuration, slog.Any("duration", elapsed.String())),
			tint.Attr(tintAttrCodeRows, slog.Any("rows", rowsAffected)),
			tint.Attr(tintAttrCodeQuery, slog.Any("query", sql)),
		)
	defer log.Release()

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
		return
	}

	if log.LevelEnabled(ctx, slog.LevelWarn) {
		if queryIsSlow {
			log.Warn("query is slow")
		}
		return
	}
}
