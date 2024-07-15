package config

import (
	"fmt"
	"path/filepath"
	"github.com/knadh/koanf/v2"
)

type FrontendWorkspacePathConfig struct {
	config *koanf.Koanf
}

func (c *FrontendWorkspacePathConfig) FrontendWorkspacePath(projectHashID string, storyHashId string) string{
	output := filepath.Join("/workspaces", projectHashID, "frontend" , ".stories" , storyHashId)
	fmt.Println("___frontend workspace service____",output)
	return output
}

func NewFrontendWorkspacePath(config *koanf.Koanf) *FrontendWorkspacePathConfig {
	return &FrontendWorkspacePathConfig{config: config}
}
