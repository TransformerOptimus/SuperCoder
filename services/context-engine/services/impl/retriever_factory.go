package impl

import (
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// NewRetriever builds the appropriate RetrieverService based on the strategy name.
func NewRetriever(
	strategy string,
	emb services.EmbedderService,
	store repositories.VectorRepository,
	graph repositories.GraphRepository,
	textRepo repositories.TextSearchRepository,
	provider services.SourceProvider,
	collection string,
	repoRoot string,
	alpha float32,
	openAICfg config.OpenAIConfig,
	logger *zap.Logger,
) services.RetrieverService {
	switch strategy {
	case "multi":
		v := NewVectorRetriever(emb, store, collection)
		k := NewKeywordRetriever(textRepo, collection)
		return NewMultiRetriever(v, k, graph, collection, logger)
	case "hybrid":
		v := NewVectorRetriever(emb, store, collection)
		g := NewGraphRetriever(graph, collection, openAICfg, logger)
		return NewHybridRetriever(v, g, alpha)
	case "graph":
		return NewGraphRetriever(graph, collection, openAICfg, logger)
	case "keyword":
		return NewKeywordRetriever(textRepo, collection)
	case "vector":
		return NewVectorRetriever(emb, store, collection)
	default:
		return NewVectorRetriever(emb, store, collection)
	}
}
