package pool

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/lmittmann/tint"
	"github.com/pitabwire/util"
	glogger "gorm.io/gorm/logger"

	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/data"
)

const (
	tintAttrCodeDuration = 214
	tintAttrCodeRows     = 12
	tintAttrCodeQuery    = 2
)

func datastoreLogger(ctx context.Context, cfg config.ConfigurationDatabaseTracing) glogger.Interface {
	logQueries := false
	slowQueryThreshold := config.DefaultSlowQueryThreshold
	if cfg != nil {
		slowQueryThreshold = cfg.GetDatabaseSlowQueryLogThreshold()
		logQueries = cfg.CanDatabaseTraceQueries()
	}

	return &dbLogger{
		logQueries:    logQueries,
		slowThreshold: slowQueryThreshold,
		baseLogger:    util.Log(ctx),
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
	baseLog := l.baseLogger.WithContext(ctx)

	queryIsSlow := elapsed > l.slowThreshold && l.slowThreshold != 0
	queryErrored := err != nil && !data.ErrorIsNoRows(err)
	shouldLog := queryErrored ||
		baseLog.Enabled(ctx, slog.LevelDebug) ||
		(baseLog.Enabled(ctx, slog.LevelInfo) && l.logQueries) ||
		(baseLog.Enabled(ctx, slog.LevelWarn) && queryIsSlow)

	if !shouldLog {
		return
	}

	sql, rows := fc()
	rowsAffected := strconv.FormatInt(rows, 10)

	log := baseLog.
		With(
			tint.Attr(tintAttrCodeDuration, slog.Any("duration", elapsed.String())),
			tint.Attr(tintAttrCodeRows, slog.Any("rows", rowsAffected)),
			tint.Attr(tintAttrCodeQuery, slog.Any("query", sql)),
		)
	defer log.Release()

	if queryIsSlow {
		log = log.WithField("SLOW Query", fmt.Sprintf(" >= %v", l.slowThreshold))
	}

	if queryErrored {
		log.WithError(err).Error(" Error running query ")
		return
	}

	if log.Enabled(ctx, slog.LevelDebug) {
		log.Debug("query executed")
		return
	}

	if log.Enabled(ctx, slog.LevelInfo) {
		if l.logQueries {
			log.Info("query executed ")
		}
		return
	}

	if log.Enabled(ctx, slog.LevelWarn) {
		if queryIsSlow {
			log.Warn("query is slow")
		}
		return
	}
}
