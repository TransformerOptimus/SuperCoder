package impl

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
)

type ShardBootstrap struct {
	shardCfg  config.ShardConfig
	openAICfg config.OpenAIConfig
	store     repositories.VectorRepository
	logger    *zap.Logger
}

func NewShardBootstrap(
	shardCfg config.ShardConfig,
	openAICfg config.OpenAIConfig,
	store repositories.VectorRepository,
	logger *zap.Logger,
) *ShardBootstrap {
	return &ShardBootstrap{
		shardCfg:  shardCfg,
		openAICfg: openAICfg,
		store:     store,
		logger:    logger.Named("shard-bootstrap"),
	}
}

func (b *ShardBootstrap) Bootstrap(ctx context.Context) error {
	prefix := b.shardCfg.ShardPrefix()
	count := b.shardCfg.ShardCount()
	dim := uint64(b.openAICfg.EmbeddingDimensions())

	b.logger.Info("Bootstrapping shard collections",
		zap.Int("count", count),
		zap.String("prefix", prefix),
		zap.Uint64("dim", dim))

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("%s_%04d", prefix, i)
		if err := b.store.EnsureCollection(ctx, name, dim); err != nil {
			b.logger.Warn("Failed to ensure shard collection",
				zap.String("collection", name),
				zap.Error(err))
		}

		if err := b.store.EnsurePayloadIndexes(ctx, name); err != nil {
			b.logger.Warn("Failed to ensure payload indexes",
				zap.String("collection", name),
				zap.Error(err))
		}
	}

	b.logger.Info("Shard bootstrap complete", zap.Int("shards", count))
	return nil
}
