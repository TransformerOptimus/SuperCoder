package config

import (
	"github.com/knadh/koanf/v2"
)

type ReviewerConfig interface {
	LLMProvider() string
	LLMModel() string
	MaxIterations() int
	MaxToolResultChars() int
	SystemPromptPath() string
	ToolsPath() string
}

type reviewerConfigImpl struct {
	config *koanf.Koanf
}

func NewReviewerConfig(config *koanf.Koanf) ReviewerConfig {
	return &reviewerConfigImpl{config: config}
}

func (c *reviewerConfigImpl) LLMProvider() string {
	return c.config.String("reviewer.provider")
}

func (c *reviewerConfigImpl) LLMModel() string {
	switch c.LLMProvider() {
	case "openai":
		return c.config.String("openai.model")
	default:
		return c.config.String("anthropic.model")
	}
}

func (c *reviewerConfigImpl) MaxIterations() int {
	return c.config.Int("reviewer.max.iterations")
}

func (c *reviewerConfigImpl) MaxToolResultChars() int {
	v := c.config.Int("reviewer.max.tool.result.chars")
	if v <= 0 {
		return 2000
	}
	return v
}

func (c *reviewerConfigImpl) SystemPromptPath() string {
	return c.config.String("agent.system.prompt.path")
}

func (c *reviewerConfigImpl) ToolsPath() string {
	return c.config.String("agent.tools.path")
}

