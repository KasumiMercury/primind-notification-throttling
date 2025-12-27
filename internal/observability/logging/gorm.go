package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type GormLogger struct {
	SlowThreshold time.Duration
	LogLevel      gormlogger.LogLevel
}

func NewGormLogger(slowThreshold time.Duration) *GormLogger {
	return &GormLogger{
		SlowThreshold: slowThreshold,
		LogLevel:      gormlogger.Info,
	}
}

func (l *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.LogLevel = level

	return &newLogger
}

func (l *GormLogger) Info(ctx context.Context, msg string, args ...any) {
	if l.LogLevel >= gormlogger.Info {
		slog.InfoContext(ctx, fmt.Sprintf(msg, args...),
			slog.String("event", "db.log"),
		)
	}
}

func (l *GormLogger) Warn(ctx context.Context, msg string, args ...any) {
	if l.LogLevel >= gormlogger.Warn {
		slog.WarnContext(ctx, fmt.Sprintf(msg, args...),
			slog.String("event", "db.log"),
		)
	}
}

func (l *GormLogger) Error(ctx context.Context, msg string, args ...any) {
	if l.LogLevel >= gormlogger.Error {
		slog.ErrorContext(ctx, fmt.Sprintf(msg, args...),
			slog.String("event", "db.log"),
		)
	}
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.LogLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	switch {
	case err != nil && l.LogLevel >= gormlogger.Error && !errors.Is(err, gorm.ErrRecordNotFound):
		slog.ErrorContext(ctx, "query error",
			slog.String("event", "db.query.fail"),
			slog.String("error", err.Error()),
			slog.Duration("duration", elapsed),
			slog.String("sql", sql),
			slog.Int64("rows", rows),
		)
	case elapsed > l.SlowThreshold && l.SlowThreshold > 0 && l.LogLevel >= gormlogger.Warn:
		slog.WarnContext(ctx, "slow query",
			slog.String("event", "db.query.slow.detect"),
			slog.Duration("duration", elapsed),
			slog.Duration("threshold", l.SlowThreshold),
			slog.String("sql", sql),
			slog.Int64("rows", rows),
		)
	case l.LogLevel >= gormlogger.Info:
		slog.DebugContext(ctx, "query executed",
			slog.String("event", "db.query"),
			slog.Duration("duration", elapsed),
			slog.String("sql", sql),
			slog.Int64("rows", rows),
		)
	}
}
