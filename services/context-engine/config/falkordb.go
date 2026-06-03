package config

import (
	"github.com/knadh/koanf/v2"
)

type FalkorDBConfig interface {
	Host() string
	Port() int
	Password() string
}

type falkorDBConfigImpl struct {
	config *koanf.Koanf
}

func NewFalkorDBConfig(config *koanf.Koanf) FalkorDBConfig {
	return &falkorDBConfigImpl{config: config}
}

func (c *falkorDBConfigImpl) Host() string {
	return c.config.String("falkordb.host")
}

func (c *falkorDBConfigImpl) Port() int {
	return c.config.Int("falkordb.port")
}

func (c *falkorDBConfigImpl) Password() string {
	return c.config.String("falkordb.password")
}
