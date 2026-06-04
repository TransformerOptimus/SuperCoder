package config

import (
	"fmt"

	"github.com/knadh/koanf/v2"
)

type ServiceConfig struct {
	config *koanf.Koanf
}

func (s ServiceConfig) Name() string {
	return s.config.String("service.name")
}

func (s ServiceConfig) Host() string {
	return s.config.String("service.host")
}

func (s ServiceConfig) Port() string {
	return s.config.String("service.port")
}

func (s ServiceConfig) Address() string {
	return fmt.Sprintf("%s:%s", s.Host(), s.Port())
}

func (s ServiceConfig) GrpcServiceName() string {
	if s.config.String("service.grpc.name") == "" {
		return s.Name()
	}
	return s.config.String("service.grpc.name")
}

func (s ServiceConfig) AuthKey() string {
	return s.config.String("service.auth.key")
}

func NewServiceConfig(config *koanf.Koanf) *ServiceConfig {
	return &ServiceConfig{
		config: config,
	}
}
