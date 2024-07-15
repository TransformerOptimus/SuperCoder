package config

func OpenAIAPIKey() string { return config.String("openai.api.key") }

func ClaudeAPIKey() string { return config.String("claude.api.key") }
