package injection

import (
	"go.uber.org/dig"
	"go.uber.org/zap"

	asynqconfig "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/asynq"
	config "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/core"
	dbconfig "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/db"
	redisconfig "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/redis"
	supercoderconfig "github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
)

func baseContainer(logger *zap.Logger) (container *dig.Container, err error) {
	container = dig.New()
	err = container.Provide(func() *zap.Logger {
		return logger
	})
	if err != nil {
		logger.Error("failed to provide logger", zap.Error(err))
		return nil, err
	}

	// Core Config
	{
		if err = container.Provide(ProvideDefaultConfig); err != nil {
			logger.Error("failed to provide default config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(config.NewConfig); err != nil {
			logger.Error("failed to provide config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(config.NewEnvConfig); err != nil {
			logger.Error("failed to provide env config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(config.NewServiceConfig); err != nil {
			logger.Error("failed to provide service config", zap.Error(err))
			return nil, err
		}
	}

	// Infrastructure Config
	{
		if err = container.Provide(dbconfig.NewDBConfig); err != nil {
			logger.Error("failed to provide db config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(redisconfig.NewRedisConfig); err != nil {
			logger.Error("failed to provide redis config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(asynqconfig.NewAsynqRedisClientOpt); err != nil {
			logger.Error("failed to provide asynq config", zap.Error(err))
			return nil, err
		}
	}

	// Service-Specific Config
	{
		if err = container.Provide(supercoderconfig.NewAnthropicConfig); err != nil {
			logger.Error("failed to provide anthropic config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(supercoderconfig.NewQdrantConfig); err != nil {
			logger.Error("failed to provide qdrant config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(supercoderconfig.NewFalkorDBConfig); err != nil {
			logger.Error("failed to provide falkordb config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(supercoderconfig.NewOpenAIConfig); err != nil {
			logger.Error("failed to provide openai config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(supercoderconfig.NewIndexerConfig); err != nil {
			logger.Error("failed to provide indexer config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(supercoderconfig.NewReviewerConfig); err != nil {
			logger.Error("failed to provide reviewer config", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(supercoderconfig.NewShardConfig); err != nil {
			logger.Error("failed to provide shard config", zap.Error(err))
			return nil, err
		}
	}

	return
}
