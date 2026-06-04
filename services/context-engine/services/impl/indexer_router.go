package impl

import (
	"context"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type indexerRouter struct {
	treeSitter services.IndexerService
	cfg        config.IndexerConfig
	logger     *zap.Logger
}

func NewIndexerRouter(cfg config.IndexerConfig, logger *zap.Logger) services.IndexerService {
	return &indexerRouter{
		treeSitter: NewTreeSitterIndexer(cfg, logger),
		cfg:        cfg,
		logger:     logger.Named("indexer-router"),
	}
}

func (r *indexerRouter) IndexDirectory(ctx context.Context, provider services.SourceProvider, root string) (*services.IndexResult, error) {
	return r.treeSitter.IndexDirectory(ctx, provider, root)
}

func (r *indexerRouter) IndexFiles(ctx context.Context, provider services.SourceProvider, root string, changedFiles []string) (*services.IndexResult, error) {
	return r.treeSitter.IndexFiles(ctx, provider, root, changedFiles)
}

func (r *indexerRouter) SupportedLanguages() []string {
	return r.cfg.SupportedLanguages()
}
