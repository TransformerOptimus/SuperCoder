package config

func SlackWebhookURL() string { return config.String("monitoring.slack.url") }
