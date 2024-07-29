package config

func GetAllowedHost() string {
	return config.String("terminal.allowed.host")
}
