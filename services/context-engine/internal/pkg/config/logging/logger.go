package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func getLogLevel() zapcore.Level {
	env := os.Getenv("SUPERAGI_ENV")
	logLevel := zap.DebugLevel
	if env == "development" || env == "staging" {
		logLevel = zap.DebugLevel
	}
	if env == "production" {
		logLevel = zap.InfoLevel
	}

	logLevelStr := strings.ToLower(os.Getenv("SUPERAGI_LOG_LEVEL"))
	if logLevelStr != "" {
		switch logLevelStr {
		case "debug":
			logLevel = zap.DebugLevel
		case "info":
			logLevel = zap.InfoLevel
		case "warn":
			logLevel = zap.WarnLevel
		case "error":
			logLevel = zap.ErrorLevel
		default:
		}
	}

	return logLevel
}

func NewLogger() (logger *zap.Logger, err error) {
	env := os.Getenv("SUPERAGI_ENV")
	logLevel := getLogLevel()
	var config zap.Config
	if env == "staging" || env == "production" {
		config = zap.Config{
			Level:       zap.NewAtomicLevelAt(logLevel),
			Development: false,
			Sampling: &zap.SamplingConfig{
				Initial:    100,
				Thereafter: 100,
			},
			Encoding:         "json",
			EncoderConfig:    zap.NewProductionEncoderConfig(),
			OutputPaths:      []string{"stderr"},
			ErrorOutputPaths: []string{"stderr"},
		}
	} else {
		config = zap.Config{
			Level:            zap.NewAtomicLevelAt(logLevel),
			Development:      true,
			Encoding:         "console",
			EncoderConfig:    zap.NewDevelopmentEncoderConfig(),
			OutputPaths:      []string{"stderr"},
			ErrorOutputPaths: []string{"stderr"},
		}
	}
	logger, err = config.Build()
	return
}
