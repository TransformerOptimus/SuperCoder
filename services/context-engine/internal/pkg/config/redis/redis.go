package redis_config

import (
	"github.com/knadh/koanf/v2"
)

type RedisConfig struct {
	config *koanf.Koanf
}

func (r RedisConfig) Host() string {
	return r.config.String("redis.host")
}

func (r RedisConfig) Port() string {
	return r.config.String("redis.port")
}

func (r RedisConfig) Addr() string {
	return r.Host() + ":" + r.Port()
}

func (r RedisConfig) DB() int {
	return r.config.Int("redis.db")
}

func (r RedisConfig) WorkerDB() int {
	return r.config.Int("redis.worker.db")
}

func (r RedisConfig) CacheDB() int {
	return r.config.Int("redis.cache.db")
}

func (r RedisConfig) CacheTTL() int {
	return r.config.Int("redis.cache.ttl")
}

func (r RedisConfig) IsEncrypted() bool {
	return r.config.Bool("redis.encrypted")
}

func NewRedisConfig(config *koanf.Koanf) RedisConfig {
	return RedisConfig{
		config: config,
	}
}
