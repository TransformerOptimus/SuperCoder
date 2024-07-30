package config

import "github.com/knadh/koanf/v2"

type GithubOAuthConfig struct {
	config *koanf.Koanf
}

func (g GithubOAuthConfig) ClientId() string {
	return g.config.String("github.client.id")
}

func (g GithubOAuthConfig) ClientSecret() string {
	return g.config.String("github.client.secret")
}

func (g GithubOAuthConfig) RedirectURL() string {
	return g.config.String("github.redirect.url")
}

func (g GithubOAuthConfig) FrontendURL() string {
	return g.config.String("github.frontend.url")
}

func NewGithubOAuthConfig(config *koanf.Koanf) *GithubOAuthConfig {
	return &GithubOAuthConfig{
		config: config,
	}
}
