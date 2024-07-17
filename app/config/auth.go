package config

func AppFrontendUrl() string { return config.String("app.frontend.url") }

func AppBackendUrl() string { return config.String("app.backend.url") }
