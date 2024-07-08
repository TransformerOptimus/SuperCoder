package config

import (
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
	"strings"
)

const Prefix = "WORKSPACES_"

// LoadConfig loads the configuration from the given file.
func LoadConfig() (config *koanf.Koanf, err error) {
	config = koanf.New(".")
	err = config.Load(confmap.Provider(map[string]interface{}{
		"env":         "dev",
		"namespace":   "workspaces",
		"values.file": "values.yaml",
		"project":     "workspace",
		"jobs": map[string]interface{}{
			"images": map[string]interface{}{},
			"docker": map[string]interface{}{
				"network": "ai-developer_default",
			},
			"local": map[string]interface{}{
				"images": map[string]interface{}{
					"python": "python-executor:latest",
					"node":   "node-executor:latest",
				},
				"autoremove": "false",
				"volume": map[string]interface{}{
					"source": "ai-developer_workspaces",
					"target": "/workspaces",
				},
			},
		},
	}, "."), nil)
	if err != nil {
		return
	}
	err = config.Load(env.Provider(Prefix, ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, Prefix)), "_", ".", -1)
	}), nil)
	return
}
