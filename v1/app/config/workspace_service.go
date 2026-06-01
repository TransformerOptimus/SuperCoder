package config

import "github.com/knadh/koanf/v2"

type WorkspaceServiceConfig struct {
	config *koanf.Koanf
}

func (wsc *WorkspaceServiceConfig) GetEndpoint() string {
	return config.String("workspace.service.endpoint")
}

func NewWorkspaceServiceConfig(config *koanf.Koanf) *WorkspaceServiceConfig {
	return &WorkspaceServiceConfig{config}
}
