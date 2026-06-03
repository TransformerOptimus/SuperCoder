package impl

import (
	"context"
	"sort"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type hybridRetriever struct {
	vector services.RetrieverService
	graph  services.RetrieverService
	alpha  float32 // 0.0 = all graph, 1.0 = all vector
}

func NewHybridRetriever(vector, graph services.RetrieverService, alpha float32) services.RetrieverService {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	return &hybridRetriever{
		vector: vector,
		graph:  graph,
		alpha:  alpha,
	}
}

func (r *hybridRetriever) Search(ctx context.Context, query string, limit int, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	vResults, vErr := r.vector.Search(ctx, query, limit, filter)
	gResults, gErr := r.graph.Search(ctx, query, limit, filter)

	if vErr != nil && gErr != nil {
		return nil, vErr
	}

	return mergeAndRank(vResults, gResults, r.alpha, limit), nil
}

func (r *hybridRetriever) FindFunction(ctx context.Context, functionName string, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	vResults, vErr := r.vector.FindFunction(ctx, functionName, filter)
	gResults, gErr := r.graph.FindFunction(ctx, functionName, filter)

	if vErr != nil && gErr != nil {
		return nil, vErr
	}

	return mergeAndRank(vResults, gResults, r.alpha, 1), nil
}

func (r *hybridRetriever) Name() string { return "hybrid" }

func mergeAndRank(vectorResults, graphResults []services.RetrieverResult, alpha float32, limit int) []services.RetrieverResult {
	type merged struct {
		result      services.RetrieverResult
		vectorScore float32
		graphScore  float32
	}

	seen := make(map[string]*merged)

	for _, r := range vectorResults {
		key := r.FilePath + "|" + r.ChunkID
		if m, ok := seen[key]; ok {
			if r.Score > m.vectorScore {
				m.vectorScore = r.Score
				m.result = r
			}
		} else {
			seen[key] = &merged{result: r, vectorScore: r.Score}
		}
	}

	for _, r := range graphResults {
		key := r.FilePath + "|" + r.ChunkID
		if m, ok := seen[key]; ok {
			if r.Score > m.graphScore {
				m.graphScore = r.Score
			}
		} else {
			seen[key] = &merged{result: r, graphScore: r.Score}
		}
	}

	var results []services.RetrieverResult
	for _, m := range seen {
		m.result.Score = alpha*m.vectorScore + (1-alpha)*m.graphScore
		m.result.Source = "hybrid"
		results = append(results, m.result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return results
}

func (r *hybridRetriever) ListFiles(ctx context.Context, filter services.SearchFilter) ([]string, error) {
	if r.vector != nil {
		return r.vector.ListFiles(ctx, filter)
	}
	return nil, nil
}

func (r *hybridRetriever) ReadFile(ctx context.Context, path string, filter services.SearchFilter) (string, error) {
	if r.vector != nil {
		return r.vector.ReadFile(ctx, path, filter)
	}
	return "", nil
}
