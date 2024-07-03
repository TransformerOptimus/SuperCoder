package config

import "go.uber.org/zap"

var Logger *zap.Logger

func InitLogger() {
	var err error
	Logger, err = zap.NewProduction()
	if err != nil {
		panic(err)
	}
}
