package impl

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type vectorRetriever struct {
	embedder   services.EmbedderService
	store      repositories.VectorRepository
	collection string
}

func NewVectorRetriever(emb services.EmbedderService, store repositories.VectorRepository, collection string) services.RetrieverService {
	return &vectorRetriever{
		embedder:   emb,
		store:      store,
		collection: collection,
	}
}

func (r *vectorRetriever) Search(ctx context.Context, query string, limit int, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	vec, err := r.embedder.EmbedSingle(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("vector retriever embed failed: %w", err)
	}

	results, err := r.store.Search(ctx, r.collection, vec, limit, filter)
	if err != nil {
		return nil, fmt.Errorf("vector retriever search failed: %w", err)
	}

	return mapVectorResults(results), nil
}

func (r *vectorRetriever) FindFunction(ctx context.Context, functionName string, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	query := fmt.Sprintf("function %s", functionName)
	vec, err := r.embedder.EmbedSingle(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("vector retriever embed failed: %w", err)
	}
	results, err := r.store.Search(ctx, r.collection, vec, 1, filter)
	if err != nil {
		return nil, fmt.Errorf("vector retriever search failed: %w", err)
	}
	return mapVectorResults(results), nil
}

func (r *vectorRetriever) Name() string { return "vector" }

func (r *vectorRetriever) ListFiles(ctx context.Context, filter services.SearchFilter) ([]string, error) {
	results, err := r.store.ListFilePaths(ctx, r.collection, filter)
	if err != nil {
		return nil, fmt.Errorf("vector retriever list files failed: %w", err)
	}
	sort.Strings(results)
	return results, nil
}

func (r *vectorRetriever) ReadFile(ctx context.Context, path string, filter services.SearchFilter) (string, error) {
	chunks, err := r.store.GetChunksByFilePath(ctx, r.collection, path)
	if err != nil {
		return "", fmt.Errorf("vector retriever read file failed: %w", err)
	}
	if len(chunks) == 0 {
		return fmt.Sprintf("File %s not found in the indexed codebase.", path), nil
	}
	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString(c)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func mapVectorResults(results []repositories.SearchResult) []services.RetrieverResult {
	out := make([]services.RetrieverResult, len(results))
	for i, r := range results {
		out[i] = services.RetrieverResult{
			ChunkID:  r.ChunkID,
			Content:  r.Content,
			FilePath: r.FilePath,
			Language: r.Language,
			Score:    r.Score,
			Source:   "vector",
		}
	}
	return out
}
