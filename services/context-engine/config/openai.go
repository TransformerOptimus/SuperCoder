package config

import (
	"github.com/knadh/koanf/v2"
)

// OpenAIConfig wraps OpenAI settings for both embeddings and chat completion.
type OpenAIConfig interface {
	BaseURL() string
	APIKey() string
	EmbeddingModel() string
	EmbeddingDimensions() int
	Model() string
	MaxTokens() int
	Temperature() float64
}

type openAIConfigImpl struct {
	config *koanf.Koanf
}

func NewOpenAIConfig(config *koanf.Koanf) OpenAIConfig {
	return &openAIConfigImpl{config: config}
}

func (c *openAIConfigImpl) BaseURL() string {
	u := c.config.String("openai.base.url")
	if u == "" {
		return "https://api.openai.com/v1"
	}
	return u
}

func (c *openAIConfigImpl) APIKey() string {
	return c.config.String("openai.api.key")
}

func (c *openAIConfigImpl) EmbeddingModel() string {
	return c.config.String("openai.embedding.model")
}

func (c *openAIConfigImpl) EmbeddingDimensions() int {
	return c.config.Int("openai.embedding.dimensions")
}

func (c *openAIConfigImpl) Model() string {
	return c.config.String("openai.model")
}

func (c *openAIConfigImpl) MaxTokens() int {
	return c.config.Int("openai.max.tokens")
}

func (c *openAIConfigImpl) Temperature() float64 {
	return c.config.Float64("openai.temperature")
}
