package config

// NewRelicLicenseKey returns the New Relic license key.
func NewRelicLicenseKey() string { return config.String("newrelic.license.key") }

// NewRelicAppName returns the New Relic app name.
func NewRelicAppName() string { return config.String("newrelic.app.name") }
