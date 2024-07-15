package config

func WorkspaceWorkingDirectory() string { return config.String("workspace.working.dir") }
func WorkspaceStaticFrontendUrl() string { return config.String("workspace.static.frontend.url") }
