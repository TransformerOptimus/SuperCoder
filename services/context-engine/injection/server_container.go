package injection

import (
	"go.uber.org/dig"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/api/crossorigin"
	asynqclient "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/clients/asynq"
	cacherclient "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/clients/cacher"
	gormlogger "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/clients/gorm"
	postgresclient "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/clients/postgres"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/controllers"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/producers"
	repoimpl "github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories/impl"
	serviceimpl "github.com/TransformerOptimus/SuperCoder/services/context-engine/services/impl"
)

func NewServerContainer(logger *zap.Logger) (container *dig.Container, err error) {
	container, err = baseContainer(logger)
	if err != nil {
		logger.Error("failed to create base container", zap.Error(err))
		return nil, err
	}

	// Cors
	{
		if err = container.Provide(crossorigin.AllowAllOrigins); err != nil {
			logger.Error("failed to provide cors", zap.Error(err))
			return nil, err
		}
	}

	// Database
	{
		if err = container.Provide(gormlogger.NewGormLogger); err != nil {
			logger.Error("failed to provide gorm logger", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(postgresclient.NewPostgresDbWithoutCache); err != nil {
			logger.Error("failed to provide postgres db", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(cacherclient.NewRedisCache); err != nil {
			logger.Error("failed to provide redis cache", zap.Error(err))
			return nil, err
		}
	}

	// Asynq
	{
		if err = container.Provide(asynqclient.NewAsyncMonHandler); err != nil {
			logger.Error("failed to provide asyncmon handler", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(asynqclient.NewAsynqClient); err != nil {
			logger.Error("failed to provide asynq client", zap.Error(err))
			return nil, err
		}
	}

	// Controllers (always registered)
	{
		if err = container.Provide(controllers.NewHealthController); err != nil {
			logger.Error("failed to provide health controller", zap.Error(err))
			return nil, err
		}
	}

	// Indexer-dependent components (gated behind indexer.enabled)
	var indexerConfig config.IndexerConfig
	if err = container.Invoke(func(cfg config.IndexerConfig) { indexerConfig = cfg }); err != nil {
		logger.Error("failed to resolve indexer config", zap.Error(err))
		return nil, err
	}

	if indexerConfig.Enabled() {
		logger.Info("Indexer enabled, registering indexer dependencies")

		// Repositories
		if err = container.Provide(repoimpl.NewVectorRepository); err != nil {
			logger.Error("failed to provide vector repository", zap.Error(err))
			return nil, err
		}
		// Wrap VectorRepository with a per-process EnsureCollection cache
		// so repeated /diff requests don't pay the ~4s Qdrant round-trip
		// on every call. The decorator self-heals on any write-path error.
		if err = container.Decorate(repoimpl.NewEnsureCacheVectorRepository); err != nil {
			logger.Error("failed to decorate vector repository with ensure cache", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(repoimpl.NewGraphRepository); err != nil {
			logger.Error("failed to provide graph repository", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(repoimpl.NewTextSearchRepository); err != nil {
			logger.Error("failed to provide text search repository", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(repoimpl.NewRepoRepository); err != nil {
			logger.Error("failed to provide repo repository", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(repoimpl.NewShardRepository); err != nil {
			logger.Error("failed to provide shard repository", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(repoimpl.NewSyncSessionRepository); err != nil {
			logger.Error("failed to provide sync session repository", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(repoimpl.NewSyncBatchRepository); err != nil {
			logger.Error("failed to provide sync batch repository", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(repoimpl.NewSyncOutboxRepository); err != nil {
			logger.Error("failed to provide sync outbox repository", zap.Error(err))
			return nil, err
		}

		// Services
		if err = container.Provide(serviceimpl.NewIndexerRouter); err != nil {
			logger.Error("failed to provide indexer service", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(serviceimpl.NewEmbedderService); err != nil {
			logger.Error("failed to provide embedder service", zap.Error(err))
			return nil, err
		}
		// WS5: cap concurrent OpenAI embed calls at 5 and classify 4xx
		// as terminal. Decorator replaces the EmbedderService binding
		// in place so retrievers + any API-side caller inherit it.
		if err = container.Decorate(serviceimpl.NewEmbedderWithSemaphore); err != nil {
			logger.Error("failed to decorate embedder with semaphore", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(serviceimpl.NewMerkleService); err != nil {
			logger.Error("failed to provide merkle service", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(ProvidePromptProvider); err != nil {
			logger.Error("failed to provide prompt provider", zap.Error(err))
			return nil, err
		}
		// Pipeline is dig-provided so the StreamingIndexer adapter can consume it.
		if err = container.Provide(serviceimpl.NewPipeline); err != nil {
			logger.Error("failed to provide pipeline", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(serviceimpl.NewShardBootstrap); err != nil {
			logger.Error("failed to provide shard bootstrap", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(serviceimpl.NewIndexRetrievalService); err != nil {
			logger.Error("failed to provide index retrieval service", zap.Error(err))
			return nil, err
		}

		// Producers
		if err = container.Provide(producers.NewIndexProducer); err != nil {
			logger.Error("failed to provide index producer", zap.Error(err))
			return nil, err
		}

		// Indexer Controllers
		if err = container.Provide(controllers.NewIndexRetrievalController); err != nil {
			logger.Error("failed to provide index retrieval controller", zap.Error(err))
			return nil, err
		}

		// Streaming sync (WS3 + WS4):
		//   - WS3: /diff + /sync-complete + TTL GC
		//   - WS4: /stream + transactional-outbox dispatcher (+ reaper)
		// Route registration and lifecycle wiring (RunTTLGCLoop /
		// OutboxDispatcher.Run) live in WS6.
		if err = container.Provide(ProvideStreamContentRedisClient); err != nil {
			logger.Error("failed to provide stream content redis client", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(ProvidePostgresDSN); err != nil {
			logger.Error("failed to provide postgres dsn", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(serviceimpl.NewSyncSessionService); err != nil {
			logger.Error("failed to provide sync session service", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(serviceimpl.NewOutboxDispatcher); err != nil {
			logger.Error("failed to provide outbox dispatcher", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(controllers.NewIndexDiffController); err != nil {
			logger.Error("failed to provide index diff controller", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(controllers.NewIndexStreamController); err != nil {
			logger.Error("failed to provide index stream controller", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(controllers.NewIndexSyncCompleteController); err != nil {
			logger.Error("failed to provide index sync complete controller", zap.Error(err))
			return nil, err
		}
	} else {
		logger.Info("Indexer disabled, skipping indexer dependencies")
	}

	return
}
