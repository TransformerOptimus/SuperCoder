package config

import "github.com/knadh/koanf/v2"

type WorkspaceJobs struct {
	config *koanf.Koanf
}

func (c *WorkspaceJobs) ContainerImage(imageName string) string {
	return c.config.String("jobs.images." + imageName)
}

func (c *WorkspaceJobs) LocalContainerImage(imageName string) string {
	return c.config.String("jobs.local.images." + imageName)
}

func (c *WorkspaceJobs) DockerNetwork() string {
	return c.config.String("jobs.docker.network")
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

func (c *WorkspaceJobs) FilestoreSource() string {
	return c.config.String("jobs.local.filestore.source")
}

func (c *WorkspaceJobs) FilestoreTarget() string {
	return c.config.String("jobs.local.filestore.target")
}

func NewWorkspaceJobs(config *koanf.Koanf) *WorkspaceJobs {
	return &WorkspaceJobs{config: config}
}
