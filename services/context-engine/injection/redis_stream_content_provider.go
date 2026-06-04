package injection

import (
	"crypto/tls"

	"github.com/redis/go-redis/v9"

	redisconfig "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/redis"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// ProvideStreamContentRedisClient is the dig provider for the typed
// wrapper services.StreamContentRedisClient. It mirrors the construction
// in pkg/clients/redis/redis.go:11 but pins DB to WorkerDB() so the API
// writer and the WS5 Asynq workers see the same Redis logical DB.
//
// The type lives in the `services` package (not here) to avoid a
// services/impl → injection import cycle. See
// services/stream_content_redis.go for the rationale.
func ProvideStreamContentRedisClient(cfg redisconfig.RedisConfig) *services.StreamContentRedisClient {
	opts := &redis.Options{
		Addr: cfg.Addr(),
		DB:   cfg.WorkerDB(),
	}
	if cfg.IsEncrypted() {
		opts.TLSConfig = &tls.Config{}
	}
	return &services.StreamContentRedisClient{Client: redis.NewClient(opts)}
}
