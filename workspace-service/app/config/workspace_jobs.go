package config

import "github.com/knadh/koanf/v2"

type WorkspaceJobs struct {
	config *koanf.Koanf
}

func (c *WorkspaceJobs) ContainerImage() string {
	return c.config.String("jobs.image")
}

func (c *WorkspaceJobs) LocalContainerImage() string {
	return c.config.String("jobs.local.image")
}

func (c *WorkspaceJobs) AutoRemoveJobContainer() bool {
	return c.config.String("jobs.local.autoremove") == "true"
}

func (c *WorkspaceJobs) VolumeSource() string {
	return c.config.String("jobs.local.volume.source")
}

func (c *WorkspaceJobs) VolumeTarget() string {
	return c.config.String("jobs.local.volume.target")
}

func NewWorkspaceJobs(config *koanf.Koanf) *WorkspaceJobs {
	return &WorkspaceJobs{config: config}
}
