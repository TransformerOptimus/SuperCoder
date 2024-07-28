package config

import (
	"github.com/knadh/koanf/v2"
	"log"
	"time"
)

type JWTConfig struct {
	config *koanf.Koanf
}

func (j JWTConfig) Secret() string {
	return j.config.String("jwt.secret.key")
}

func (j JWTConfig) ExpiryHours() time.Duration {
	expiryHours := j.config.String("jwt.expiry.hours")
	expiryDuration, err := time.ParseDuration(expiryHours)
	if err != nil {
		log.Fatalf("could not parse jwt.expiry_hours: %v", err)
	}
	return expiryDuration
}

func NewJWTConfig(config *koanf.Koanf) *JWTConfig {
	return &JWTConfig{config: config}
}

func JWTSecret() string { return config.String("jwt.secret.key") }

func JWTExpiryHours() time.Duration {
	expiryHours := config.String("jwt.expiry.hours")
	expiryDuration, err := time.ParseDuration(expiryHours)
	if err != nil {
		log.Fatalf("could not parse jwt.expiry_hours: %v", err)
	}
	return expiryDuration
}
