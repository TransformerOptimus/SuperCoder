package config

import (
	"ai-developer/app/constants"
	"go.uber.org/zap"
	"os"
)

var Logger *zap.Logger

func InitLogger() {
	var err error
	if env, exists := os.LookupEnv("AI_DEVELOPER_APP_ENV"); env != constants.Production || !exists {
		Logger, err = zap.NewDevelopment()
		if err != nil {
			panic(err)
		}
		Logger.Info("Logger initialized in development mode")
	} else {
		Logger, err = zap.NewProduction()
		if err != nil {
			panic(err)
		}
		Logger.Info("Logger initialized in production mode")
	}
}
