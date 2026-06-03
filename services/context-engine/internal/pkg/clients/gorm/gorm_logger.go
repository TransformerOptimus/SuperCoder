package gormlogger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm/logger"
)

// GormLogger wraps zap.Logger to implement GORM's logger.Interface
type GormLogger struct {
	zapLogger                 *zap.Logger
	logLevel                  logger.LogLevel
	slowThreshold             time.Duration
	ignoreRecordNotFoundError bool
}

// NewGormLogger creates a new GORM logger that uses zap as the underlying logger
func NewGormLogger(zapLogger *zap.Logger) logger.Interface {
	return &GormLogger{
		zapLogger:                 zapLogger,
		logLevel:                  logger.Warn,
		slowThreshold:             200 * time.Millisecond,
		ignoreRecordNotFoundError: true,
	}
}

// LogMode sets the log level
func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newLogger := *l
	newLogger.logLevel = level
	return &newLogger
}

// Info logs info level messages
func (l *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Info {
		l.zapLogger.Info(fmt.Sprintf(msg, data...))
	}
}

// Warn logs warn level messages
func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Warn {
		l.zapLogger.Warn(fmt.Sprintf(msg, data...))
	}
}

// Error logs error level messages
func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Error {
		l.zapLogger.Error(fmt.Sprintf(msg, data...))
	}
}

// Trace logs SQL queries and their execution time
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.logLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := []zap.Field{
		zap.String("sql", sql),
		zap.Duration("elapsed", elapsed),
		zap.Int64("rows", rows),
	}

	switch {
	case err != nil && l.logLevel >= logger.Error && (!errors.Is(err, logger.ErrRecordNotFound) || !l.ignoreRecordNotFoundError):
		l.zapLogger.Error("SQL query failed", append(fields, zap.Error(err))...)
	case elapsed > l.slowThreshold && l.slowThreshold != 0 && l.logLevel >= logger.Warn:
		l.zapLogger.Warn("Slow SQL query", fields...)
	case l.logLevel == logger.Info:
		l.zapLogger.Info("SQL query", fields...)
	}
}
