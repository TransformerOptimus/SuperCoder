package config

func PostmarkBaseURL() string { return config.String("postmark.base.url") }

func PostmarkFromEmail() string { return config.String("postmark.from.email") }

func PostmarkOutboundServerToken() string { return config.String("postmark.outbound.server.token") }
