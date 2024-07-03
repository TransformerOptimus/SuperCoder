package config

func GitnessURL() string {
	return config.String("gitness.url")
}

func GitnessUser() string {
	return config.String("gitness.user")
}

func GitnessToken() string {
	return config.String("gitness.token")
}

func GitnessHost() string {
	return config.String("gitness.host")
}
