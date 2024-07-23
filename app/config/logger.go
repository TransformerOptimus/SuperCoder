package config

import "go.uber.org/zap"

var Logger *zap.Logger

func InitLogger() {
	if AppEnv() == "development" {
		Logger, _ = zap.NewDevelopment(zap.IncreaseLevel(zap.DebugLevel))
	} else {
		Logger, _ = zap.NewProduction()
	}
}
