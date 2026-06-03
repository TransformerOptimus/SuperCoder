package config

import (
	"github.com/knadh/koanf/v2"
)

type AnthropicConfig interface {
	APIKey() string
	Model() string
	MaxTokens() int
	Temperature() float64
}

type anthropicConfigImpl struct {
	config *koanf.Koanf
}

func NewAnthropicConfig(config *koanf.Koanf) AnthropicConfig {
	return &anthropicConfigImpl{config: config}
}

func (c *anthropicConfigImpl) APIKey() string {
	return c.config.String("anthropic.api.key")
}

func (c *anthropicConfigImpl) Model() string {
	return c.config.String("anthropic.model")
}

func (c *anthropicConfigImpl) MaxTokens() int {
	return c.config.Int("anthropic.max.tokens")
}

func (c *anthropicConfigImpl) Temperature() float64 {
	return c.config.Float64("anthropic.temperature")
}
