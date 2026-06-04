package db_config

import (
	"github.com/knadh/koanf/v2"
)

type DBConfig struct {
	config *koanf.Koanf
}

func (r DBConfig) Host() string {
	return r.config.String("db.host")
}

func (r DBConfig) Port() string {
	return r.config.String("db.port")
}

func (r DBConfig) User() string {
	return r.config.String("db.user")
}

func (r DBConfig) Password() string {
	return r.config.String("db.password")
}

func (r DBConfig) DBName() string {
	return r.config.String("db.name")
}

func (r DBConfig) IsSSL() bool {
	return r.config.Bool("db.ssl")
}

func NewDBConfig(config *koanf.Koanf) DBConfig {
	return DBConfig{
		config: config,
	}
}
