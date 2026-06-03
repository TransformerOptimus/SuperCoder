package injection

import (
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/dig"
	"go.uber.org/zap"

	asynqclient "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/clients/asynq"
	cacherclient "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/clients/cacher"
	gormlogger "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/clients/gorm"
	postgresclient "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/clients/postgres"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/consumers"
	repoimpl "github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories/impl"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
	serviceimpl "github.com/TransformerOptimus/SuperCoder/services/context-engine/services/impl"
)

func NewWorkerContainer(logger *zap.Logger) (container *dig.Container, err error) {
	container, err = baseContainer(logger)
	if err != nil {
		logger.Error("failed to create base container", zap.Error(err))
		return nil, err
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

	// Asynq Providers
	{
		if err = container.Provide(func(opt asynq.RedisClientOpt, l *zap.Logger) *asynq.Server {
			// Streaming/finalize tasks need a longer drain window than the
			// 8s default — 60s matches the typical worst-case task duration.
			return asynqclient.NewAsynqServerWithShutdownTimeout(opt, l, 60*time.Second)
		}); err != nil {
			logger.Error("failed to provide asynq server", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(asynqclient.NewAsynqServerMux); err != nil {
			logger.Error("failed to provide asynq server mux", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(asynqclient.NewAsynqScheduler); err != nil {
			logger.Error("failed to provide asynq scheduler", zap.Error(err))
			return nil, err
		}

		if err = container.Provide(asynqclient.NewAsynqClient); err != nil {
			logger.Error("failed to provide asynq client", zap.Error(err))
			return nil, err
		}
	}

	// Resolve indexer config to decide pipeline mode (indexer.enabled).
	var indexerConfig config.IndexerConfig
	if err = container.Invoke(func(cfg config.IndexerConfig) { indexerConfig = cfg }); err != nil {
		logger.Error("failed to resolve indexer config", zap.Error(err))
		return nil, err
	}

	// Prompt provider — needed by the pipeline.
	if err = container.Provide(ProvidePromptProvider); err != nil {
		logger.Error("failed to provide prompt provider", zap.Error(err))
		return nil, err
	}

	if indexerConfig.Enabled() {
		logger.Info("Indexer enabled, registering full pipeline")

		// Repositories
		if err = container.Provide(repoimpl.NewVectorRepository); err != nil {
			logger.Error("failed to provide vector repository", zap.Error(err))
			return nil, err
		}
		// Wrap VectorRepository with a per-process EnsureCollection cache
		// so repeated /diff requests don't pay the ~4s Qdrant round-trip on
		// every call. The decorator self-heals on any write-path error.
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
		// Streaming sync repositories — the worker needs the same
		// sync_sessions/sync_batches/sync_outbox surface the /stream API
		// uses, so the Asynq handlers can read/update session state.
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
		// Cap concurrent OpenAI embed calls and classify 4xx as terminal.
		// Decorator replaces the EmbedderService binding in place; every
		// consumer (pipeline, retrievers) inherits it without call-site changes.
		if err = container.Decorate(serviceimpl.NewEmbedderWithSemaphore); err != nil {
			logger.Error("failed to decorate embedder with semaphore", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(serviceimpl.NewMerkleService); err != nil {
			logger.Error("failed to provide merkle service", zap.Error(err))
			return nil, err
		}
		// Pipeline must be dig-provided because the streaming worker consumes
		// it via the StreamingIndexer adapter below.
		if err = container.Provide(serviceimpl.NewPipeline); err != nil {
			logger.Error("failed to provide pipeline", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(serviceimpl.NewShardBootstrap); err != nil {
			logger.Error("failed to provide shard bootstrap", zap.Error(err))
			return nil, err
		}

		// Streaming worker wiring.
		//   - StreamContentRedisClient: pinned to RedisConfig.WorkerDB()
		//     (same DB as Asynq — mandatory). Shared with server_container.go.
		//   - StreamingIndexer: narrow adapter over *Pipeline so the
		//     stream_batch consumer can be mocked in tests.
		//   - FinalizerTrigger: runs the processing→finalizing flip after
		//     each batch terminal transition.
		//   - StreamBatchConsumer / FinalizeSyncConsumer: the Asynq handlers.
		if err = container.Provide(ProvideStreamContentRedisClient); err != nil {
			logger.Error("failed to provide stream content redis client", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(func(p *serviceimpl.Pipeline) services.StreamingIndexer {
			return p
		}); err != nil {
			logger.Error("failed to provide streaming indexer adapter", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(consumers.NewFinalizerTrigger); err != nil {
			logger.Error("failed to provide finalizer trigger", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(consumers.NewStreamBatchConsumer); err != nil {
			logger.Error("failed to provide stream batch consumer", zap.Error(err))
			return nil, err
		}
		if err = container.Provide(consumers.NewFinalizeSyncConsumer); err != nil {
			logger.Error("failed to provide finalize sync consumer", zap.Error(err))
			return nil, err
		}

		// Register the v3 streaming Asynq handlers behind the
		// indexer.streaming.enabled sub-flag.
		if indexerConfig.StreamingEnabled() {
			err = container.Invoke(func(
				serveMux *asynq.ServeMux,
				streamBatch *consumers.StreamBatchConsumer,
				finalizeSync *consumers.FinalizeSyncConsumer,
			) {
				serveMux.HandleFunc(services.TaskTypeStreamBatch, streamBatch.Handle)
				serveMux.HandleFunc(services.TaskTypeFinalize, finalizeSync.Handle)
				logger.Info("Registered streaming task handlers",
					zap.String("stream_batch", services.TaskTypeStreamBatch),
					zap.String("finalize", services.TaskTypeFinalize))
			})
			if err != nil {
				logger.Error("failed to register streaming task handlers", zap.Error(err))
				return nil, err
			}
		} else {
			logger.Info("Streaming sync disabled, streaming task handlers not registered")
		}
	} else {
		logger.Info("Indexer disabled, no task handlers registered")
	}

	return
}
