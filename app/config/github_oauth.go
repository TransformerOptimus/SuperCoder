package config

func GithubClientId() string { return config.String("github.client.id") }

func GithubClientSecret() string { return config.String("github.client.secret") }

func GithubRedirectURL() string { return config.String("github.redirect.url") }

func GithubFrontendURL() string { return config.String("github.frontend.url") }
