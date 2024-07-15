package config

import (
	"fmt"
	"path/filepath"
	"github.com/knadh/koanf/v2"
)

type FrontendWorkspaceConfig struct {
	config *koanf.Koanf
}

func (c *FrontendWorkspaceConfig) FrontendWorkspacePath(projectHashID string, storyHashId string) string{
	output := filepath.Join("/workspaces", projectHashID, "frontend" , ".stories" , storyHashId)
	fmt.Println("___frontend workspace service____",output)
	return output
}

func NewFrontendWorkspaceConfig(config *koanf.Koanf) *FrontendWorkspaceConfig {
	return &FrontendWorkspaceConfig{config: config}
}
