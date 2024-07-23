package config

import "github.com/knadh/koanf/v2"

type GithubIntegrationConfig struct {
	config *koanf.Koanf
}

func (gic *GithubIntegrationConfig) GetClientID() string {
	return config.String("github.integration.client.id")
}

func (gic *GithubIntegrationConfig) GetClientSecret() string {
	return config.String("github.integration.client.secret")
}

func (gic *GithubIntegrationConfig) GetRedirectURL() string {
	return config.String("github.integration.client.redirecturl")
}

func NewGithubIntegrationConfig(config *koanf.Koanf) *GithubIntegrationConfig {
	return &GithubIntegrationConfig{config}
}
