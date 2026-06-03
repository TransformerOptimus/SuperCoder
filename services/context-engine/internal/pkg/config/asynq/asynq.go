package asynq_config

import (
	"github.com/hibiken/asynq"

	config "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/redis"
)

func NewAsynqRedisClientOpt(redisConfig config.RedisConfig) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr: redisConfig.Addr(),
		DB:   redisConfig.WorkerDB(),
	}
}
