package config

import (
	"log"
	"time"
)

func JWTSecret() string { return config.String("jwt.secret.key") }

func JWTExpiryHours() time.Duration {
	expiryHours := config.String("jwt.expiry.hours")
	expiryDuration, err := time.ParseDuration(expiryHours)
	if err != nil {
		log.Fatalf("could not parse jwt.expiry_hours: %v", err)
	}
	return expiryDuration
}
