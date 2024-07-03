package config

import "github.com/knadh/koanf/v2"

type NewRelicConfig struct {
	config *koanf.Koanf
}

func (c *NewRelicConfig) LicenseKey() string {
	return c.config.String("newrelic.license.key")
}

func (c *NewRelicConfig) AppName() string {
	return c.config.String("newrelic.app.name")
}

func NewNewRelicConfig(config *koanf.Koanf) *NewRelicConfig {
	return &NewRelicConfig{config: config}
}
