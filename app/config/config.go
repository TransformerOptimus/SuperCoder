package config

import (
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
	"strings"
)

const Prefix = "AI_DEVELOPER_"

var config = koanf.New(".")

func LoadConfig() (*koanf.Koanf, error) {
	// Load default configurations
	err := config.Load(confmap.Provider(map[string]interface{}{
		"app.env":                    "development",
		"db.host":                    "localhost",
		"db.user":                    "postgres",
		"db.password":                "postgres",
		"db.name":                    "ai-developer",
		"db.port":                    5432,
		"redis.host":                 "localhost",
		"redis.port":                 6379,
		"redis.db":                   0,
		"github.redirect.url":        "http://localhost:3000/api/github/callback",
		"github.frontend.url":        "http://localhost:3000",
		"jwt.secret.key":             "asdlajksdjaskdajskdlasd",
		"jwt.expiry.hours":           "200h",
		"workspace.service.endpoint": "http://ws:8080",
		"workspace": map[string]interface{}{
			"working": map[string]interface{}{
				"dir": "/workspaces",
			},
		},
	}, "."), nil)
	if err != nil {
		return nil, err
	}

	// Load configurations from environment variables
	err = config.Load(env.Provider(Prefix, ".", func(s string) string {
		return strings.Replace(strings.ToLower(strings.TrimPrefix(s, Prefix)), "_", ".", -1)
	}), nil)
	if err != nil {
		return nil, err
	}
	return config, err
}

// Get returns the value for a given key.
func Get(key string) interface{} {
	return config.Get(key)
}
