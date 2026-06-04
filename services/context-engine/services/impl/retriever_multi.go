package impl

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type multiRetriever struct {
	vector    services.RetrieverService
	keyword   services.RetrieverService
	graph     repositories.GraphRepository
	graphName string
	logger    *zap.Logger
}

func NewMultiRetriever(
	vector, keyword services.RetrieverService,
	graph repositories.GraphRepository,
	graphName string,
	logger *zap.Logger,
) services.RetrieverService {
	return &multiRetriever{
		vector:    vector,
		keyword:   keyword,
		graph:     graph,
		graphName: graphName,
		logger:    logger.Named("multi-retriever"),
	}
}

func (r *multiRetriever) Search(ctx context.Context, query string, limit int, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	// Run vector + keyword in parallel.
	g, gCtx := errgroup.WithContext(ctx)
	var vectorResults, keywordResults []services.RetrieverResult
	var vectorErr, keywordErr error

	g.Go(func() error {
		vectorResults, vectorErr = r.vector.Search(gCtx, query, limit, filter)
		return nil
	})
	g.Go(func() error {
		keywordResults, keywordErr = r.keyword.Search(gCtx, query, limit, filter)
		return nil
	})
	_ = g.Wait()

	// Concat + dedup by FilePath|ChunkID.
	results, seenFiles := dedupResults(vectorResults, keywordResults)

	if len(results) == 0 {
		if vectorErr != nil {
			return nil, fmt.Errorf("all retrievers failed, first error: %w", vectorErr)
		}
		if keywordErr != nil {
			return nil, fmt.Errorf("all retrievers failed, first error: %w", keywordErr)
		}
	}

	// Graph expansion on matched files.
	if r.graph != nil && len(seenFiles) > 0 {
		graphResults := r.expandViaGraph(ctx, seenFiles, filter)
		results = append(results, graphResults...)
	}

	return results, nil
}

// expandViaGraph takes file paths from search results, finds all functions in those files,
// and traverses callers (blast radius) and dependencies for each function.
func (r *multiRetriever) expandViaGraph(ctx context.Context, files map[string]bool, filter services.SearchFilter) []services.RetrieverResult {
	// Get all functions in matched files.
	type funcTarget struct{ name, filePath string }
	seenFuncs := make(map[string]bool)
	var targets []funcTarget

	for filePath := range files {
		funcs, err := r.graph.GetFunctionsByFile(ctx, r.graphName, filePath, filter)
		if err != nil {
			continue
		}
		for _, f := range funcs {
			key := f.Name + "|" + f.FilePath
			if !seenFuncs[key] {
				seenFuncs[key] = true
				targets = append(targets, funcTarget{f.Name, f.FilePath})
			}
		}
	}

	if len(targets) == 0 {
		return nil
	}

	r.logger.Debug("Graph expansion",
		zap.Int("files", len(files)),
		zap.Int("functions", len(targets)))

	// Traverse callers and dependencies for each function.
	seen := make(map[string]bool)
	var allResults []services.RetrieverResult

	for _, t := range targets {
		callers, bErr := r.graph.GetBlastRadius(ctx, r.graphName, t.name, t.filePath, filter)
		if bErr == nil {
			for _, c := range callers {
				key := c.Name + "|" + c.FilePath
				if !seen[key] {
					seen[key] = true
					allResults = append(allResults, graphResultToRetriever(c))
				}
			}
		}

		deps, dErr := r.graph.GetDependencies(ctx, r.graphName, t.name, t.filePath, filter)
		if dErr == nil {
			for _, d := range deps {
				key := d.Name + "|" + d.FilePath
				if !seen[key] {
					seen[key] = true
					allResults = append(allResults, graphResultToRetriever(d))
				}
			}
		}
	}

	return allResults
}

func graphResultToRetriever(r repositories.GraphResult) services.RetrieverResult {
	return services.RetrieverResult{
		ChunkID:  r.ChunkID,
		Content:  fmt.Sprintf("%s (depth: %d)", r.Name, r.Depth),
		FilePath: r.FilePath,
		Score:    1.0 / float32(r.Depth+1),
		Source:   "graph",
	}
}

func (r *multiRetriever) FindFunction(ctx context.Context, functionName string, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	g, gCtx := errgroup.WithContext(ctx)
	var vectorResults, keywordResults []services.RetrieverResult
	var vectorErr, keywordErr error

	g.Go(func() error {
		vectorResults, vectorErr = r.vector.FindFunction(gCtx, functionName, filter)
		return nil
	})
	g.Go(func() error {
		keywordResults, keywordErr = r.keyword.FindFunction(gCtx, functionName, filter)
		return nil
	})
	_ = g.Wait()

	results, _ := dedupResults(vectorResults, keywordResults)

	if len(results) == 0 {
		if vectorErr != nil {
			return nil, fmt.Errorf("all retrievers failed, first error: %w", vectorErr)
		}
		if keywordErr != nil {
			return nil, fmt.Errorf("all retrievers failed, first error: %w", keywordErr)
		}
	}

	return results, nil
}

func (r *multiRetriever) Name() string { return "multi" }

// dedupResults concatenates two result slices and deduplicates by FilePath|ChunkID.
// Returns the deduped results and a set of unique file paths.
func dedupResults(a, b []services.RetrieverResult) ([]services.RetrieverResult, map[string]bool) {
	seen := make(map[string]bool)
	seenFiles := make(map[string]bool)
	var out []services.RetrieverResult

	for _, results := range [][]services.RetrieverResult{a, b} {
		for _, r := range results {
			key := r.FilePath + "|" + r.ChunkID
			if !seen[key] {
				seen[key] = true
				out = append(out, r)
				if r.FilePath != "" {
					seenFiles[r.FilePath] = true
				}
			}
		}
	}

	return out, seenFiles
}

func (r *multiRetriever) ListFiles(ctx context.Context, filter services.SearchFilter) ([]string, error) {
	// Use vector retriever to list files if available
	if r.vector != nil {
		return r.vector.ListFiles(ctx, filter)
	}
	return nil, fmt.Errorf("no vector retriever configured")
}

func (r *multiRetriever) ReadFile(ctx context.Context, path string, filter services.SearchFilter) (string, error) {
	// Use vector retriever to read file chunks
	if r.vector != nil {
		return r.vector.ReadFile(ctx, path, filter)
	}
	return "", fmt.Errorf("no vector retriever configured")
}
