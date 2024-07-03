package config

import "github.com/knadh/koanf/v2"

type WorkspaceServiceConfig struct {
	config *koanf.Koanf
}

func (c *WorkspaceServiceConfig) GitnessAuthUsername() string {

	return c.config.String("gitness.user")
}

func (c *WorkspaceServiceConfig) GitnessAuthToken() string {
	return c.config.String("gitness.token")
}

func (c *WorkspaceServiceConfig) WorkspaceHostName() string {
	return c.config.String("host")
}

func (c *WorkspaceServiceConfig) WorkspaceNamespace() string {
	return c.config.String("namespace")
}

func (c *WorkspaceServiceConfig) WorkspaceValuesFileName() string {
	return c.config.String("values.file")
}

func (c *WorkspaceServiceConfig) WorkspaceProject() string {
	return c.config.String("project")
}

func (c *WorkspaceServiceConfig) ArgoRepoUrl() string {
	return c.config.String("argo.repo.url")
}

func NewWorkspaceService(config *koanf.Koanf) *WorkspaceServiceConfig {
	return &WorkspaceServiceConfig{config: config}
}
