package impl

import (
	"context"
	"sort"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type keywordRetriever struct {
	textRepo   repositories.TextSearchRepository
	collection string
}

// NewKeywordRetriever creates a BM25-based keyword retriever.
func NewKeywordRetriever(textRepo repositories.TextSearchRepository, collection string) services.RetrieverService {
	return &keywordRetriever{
		textRepo:   textRepo,
		collection: collection,
	}
}

func (r *keywordRetriever) Search(ctx context.Context, query string, limit int, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	results, err := r.textRepo.Search(ctx, r.collection, query, limit, filter)
	if err != nil {
		return nil, err
	}

	out := make([]services.RetrieverResult, 0, len(results))
	for _, res := range results {
		out = append(out, services.RetrieverResult{
			ChunkID:  res.ChunkID,
			Content:  res.Content,
			FilePath: res.FilePath,
			Language: res.Language,
			Score:    float32(res.Score),
			Source:   "keyword",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *keywordRetriever) FindFunction(ctx context.Context, functionName string, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	return r.Search(ctx, functionName, 5, filter)
}

func (r *keywordRetriever) Name() string { return "keyword" }

func (r *keywordRetriever) ListFiles(_ context.Context, _ services.SearchFilter) ([]string, error) {
	return nil, nil
}

func (r *keywordRetriever) ReadFile(_ context.Context, _ string, _ services.SearchFilter) (string, error) {
	return "", nil
}
