package config

import "github.com/knadh/koanf/v2"

// Deprecated
func GithubClientId() string { return config.String("github.client.id") }

// Deprecated
func GithubClientSecret() string { return config.String("github.client.secret") }

// Deprecated
func GithubRedirectURL() string { return config.String("github.redirect.url") }

// Deprecated
func GithubFrontendURL() string { return config.String("github.frontend.url") }

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
