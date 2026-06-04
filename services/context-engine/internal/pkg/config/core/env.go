package config

import "github.com/knadh/koanf/v2"

type EnvConfig struct {
	config *koanf.Koanf
}

func (e *EnvConfig) IsProduction() bool {
	return e.Environment() == "production"
}

func (e *EnvConfig) IsStaging() bool {
	return e.Environment() == "staging"
}

func (e *EnvConfig) IsDevelopment() bool {
	return e.Environment() == "development"
}

func (e *EnvConfig) Environment() string {
	return e.config.MustString("env")
}

func NewEnvConfig(config *koanf.Koanf) *EnvConfig {
	return &EnvConfig{config: config}
}
