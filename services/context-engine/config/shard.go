package config

import (
	"github.com/knadh/koanf/v2"
)

type ShardConfig interface {
	ShardCount() int
	ShardPrefix() string
}

type shardConfigImpl struct {
	config *koanf.Koanf
}

func NewShardConfig(config *koanf.Koanf) ShardConfig {
	return &shardConfigImpl{config: config}
}

func (c *shardConfigImpl) ShardCount() int {
	return c.config.Int("qdrant.shard.count")
}

func (c *shardConfigImpl) ShardPrefix() string {
	return c.config.String("qdrant.shard.prefix")
}
