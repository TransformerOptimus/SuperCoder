package config

import (
	"github.com/knadh/koanf/v2"
)

type GRPCConfig interface {
	CoderIntegrationsHost() string
	CoderIntegrationsPort() int
	CoderIntegrationsAuthKey() string
	CoderIntegrationsTimeout() int
}

type grpcConfigImpl struct {
	config *koanf.Koanf
}

func NewGRPCConfig(config *koanf.Koanf) GRPCConfig {
	return &grpcConfigImpl{config: config}
}

func (c *grpcConfigImpl) CoderIntegrationsHost() string {
	return c.config.String("grpc.coder.integrations.host")
}

func (c *grpcConfigImpl) CoderIntegrationsPort() int {
	return c.config.Int("grpc.coder.integrations.port")
}

func (c *grpcConfigImpl) CoderIntegrationsAuthKey() string {
	return c.config.String("grpc.coder.integrations.auth.key")
}

func (c *grpcConfigImpl) CoderIntegrationsTimeout() int {
	return c.config.Int("grpc.coder.integrations.timeout")
}
