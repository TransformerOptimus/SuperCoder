package config

import (
	"github.com/knadh/koanf/v2"
)

type QdrantConfig interface {
	Host() string
	Port() int
}

type qdrantConfigImpl struct {
	config *koanf.Koanf
}

func NewQdrantConfig(config *koanf.Koanf) QdrantConfig {
	return &qdrantConfigImpl{config: config}
}

func (c *qdrantConfigImpl) Host() string {
	return c.config.String("qdrant.host")
}

func (c *qdrantConfigImpl) Port() int {
	return c.config.Int("qdrant.port")
}
