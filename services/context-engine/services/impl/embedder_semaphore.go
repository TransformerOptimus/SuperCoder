package impl

import (
	"context"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// embedderSemaphore wraps an EmbedderService to bound concurrent calls and
// classify terminal OpenAI errors. It is installed via dig.Decorate at
// container build time (see injection/server_container.go and
// injection/worker_container.go) so every caller of EmbedderService — the
// WS5 streaming pipeline, retrievers, any ad-hoc use — inherits the cap and
// the terminal-error wrapping without any call-site changes.
//
// Two-birds wrapping: the semaphore wouldn't strictly need to classify
// errors, but since it's the single place every Embed call flows through,
// it's also the cheapest place to stamp a stable Reason onto OpenAI 4xx
// failures so the WS5 worker can route them via markBatchTerminallyFailed.
type embedderSemaphore struct {
	inner  services.EmbedderService
	sem    chan struct{}
	logger *zap.Logger
}

// NewEmbedderWithSemaphore returns an EmbedderService that caps concurrent
// Embed / EmbedSingle calls at indexerCfg.EmbeddingConcurrency() and wraps
// OpenAI 4xx errors as services.Terminal("openai_embed", ...). Registered
// as a dig decorator so dig.Provide(NewEmbedderService) is unchanged; the
// decorator replaces the resolved EmbedderService after construction. The
// config dependency resolves automatically via the container since
// IndexerConfig is already provided.
func NewEmbedderWithSemaphore(inner services.EmbedderService, logger *zap.Logger, indexerCfg config.IndexerConfig) services.EmbedderService {
	limit := indexerCfg.EmbeddingConcurrency()
	return &embedderSemaphore{
		inner:  inner,
		sem:    make(chan struct{}, limit),
		logger: logger.Named("embedder-semaphore"),
	}
}

func (e *embedderSemaphore) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	out, err := e.inner.Embed(ctx, texts)
	if err != nil && services.IsTerminalEmbedError(err) {
		return nil, services.Terminal("openai_embed", err)
	}
	return out, err
}

func (e *embedderSemaphore) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	out, err := e.inner.EmbedSingle(ctx, text)
	if err != nil && services.IsTerminalEmbedError(err) {
		return nil, services.Terminal("openai_embed", err)
	}
	return out, err
}
