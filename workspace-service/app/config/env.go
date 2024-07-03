package config

import "github.com/knadh/koanf/v2"

type EnvConfig struct {
	config *koanf.Koanf
}

func (c *EnvConfig) GetEnv() string {
	return c.config.String("env")
}

func (c *EnvConfig) IsDev() bool {
	return c.config.String("env") == "dev"
}

func NewEnvConfig(config *koanf.Koanf) *EnvConfig {
	return &EnvConfig{config: config}
}

