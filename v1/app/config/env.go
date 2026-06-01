package config

import (
	"ai-developer/app/constants"
	"github.com/knadh/koanf/v2"
	"go.uber.org/zap"
)

type EnvConfig struct {
	config *koanf.Koanf
	logger *zap.Logger
}

func (e EnvConfig) IsDevelopment() bool {
	return e.config.String("app.env") == constants.Development
}

func (e EnvConfig) Domain() string {
	var domain string
	if !e.IsDevelopment() {
		domain = e.config.String("domain")
	}
	e.logger.Debug("Setting domain for auth", zap.String("domain", domain))
	return domain
}

func NewEnvConfig(config *koanf.Koanf, logger *zap.Logger) *EnvConfig {
	return &EnvConfig{
		config: config,
		logger: logger.Named("EnvConfig"),
	}
}
