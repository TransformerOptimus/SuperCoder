package asynq_client

import (
	"fmt"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

// zapAsynqLogger is an adapter that implements asynq.Logger interface using zap.Logger
type zapAsynqLogger struct {
	logger *zap.Logger
}

// Debug logs a message at Debug level.
func (l *zapAsynqLogger) Debug(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

// Info logs a message at Info level.
func (l *zapAsynqLogger) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

// Warn logs a message at Warn level.
func (l *zapAsynqLogger) Warn(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

// Error logs a message at Error level.
func (l *zapAsynqLogger) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

// Fatal logs a message at Fatal level and terminates the program.
func (l *zapAsynqLogger) Fatal(args ...interface{}) {
	l.logger.Fatal(fmt.Sprint(args...))
}

func NewZapAsynqLogger(logger *zap.Logger) asynq.Logger {
	return &zapAsynqLogger{logger: logger}
}
