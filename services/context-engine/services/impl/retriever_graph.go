package impl

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type graphRetriever struct {
	graph     repositories.GraphRepository
	graphName string
	openAICfg config.OpenAIConfig
	logger    *zap.Logger
}

func NewGraphRetriever(graph repositories.GraphRepository, graphName string, openAICfg config.OpenAIConfig, logger *zap.Logger) services.RetrieverService {
	return &graphRetriever{
		graph:     graph,
		graphName: graphName,
		openAICfg: openAICfg,
		logger:    logger.Named("graph-retriever"),
	}
}

func (r *graphRetriever) Search(ctx context.Context, query string, limit int, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	// Extract meaningful keywords from the natural language query via LLM.
	keywords, err := extractGraphKeywords(ctx, r.openAICfg, query)
	if err != nil {
		r.logger.Warn("Keyword extraction failed, skipping graph search", zap.Error(err))
		return nil, nil
	}
	if keywords == "" {
		return nil, nil
	}

	r.logger.Debug("Graph search keywords", zap.String("original", query), zap.String("keywords", keywords))

	funcs, err := r.graph.SearchFunctions(ctx, r.graphName, keywords, filter, 10)
	if err != nil {
		return nil, fmt.Errorf("graph retriever search failed: %w", err)
	}

	// For each matched function, get callers and dependencies.
	seen := make(map[string]bool)
	var allResults []repositories.GraphResult

	for _, f := range funcs {
		deps, dErr := r.graph.GetDependencies(ctx, r.graphName, f.Name, f.FilePath, filter)
		if dErr == nil {
			for _, d := range deps {
				key := d.Name + "|" + d.FilePath
				if !seen[key] {
					seen[key] = true
					allResults = append(allResults, d)
				}
			}
		}

		callers, bErr := r.graph.GetBlastRadius(ctx, r.graphName, f.Name, f.FilePath, filter)
		if bErr == nil {
			for _, c := range callers {
				key := c.Name + "|" + c.FilePath
				if !seen[key] {
					seen[key] = true
					allResults = append(allResults, c)
				}
			}
		}
	}

	return mapGraphResults(allResults, limit), nil
}

func (r *graphRetriever) FindFunction(ctx context.Context, functionName string, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	deps, err := r.graph.GetDependencies(ctx, r.graphName, functionName, "", filter)
	if err != nil {
		return nil, fmt.Errorf("graph retriever find function failed: %w", err)
	}

	callers, err := r.graph.GetBlastRadius(ctx, r.graphName, functionName, "", filter)
	if err != nil {
		return mapGraphResults(deps, 5), nil
	}

	combined := append(deps, callers...)
	return mapGraphResults(combined, 5), nil
}

func (r *graphRetriever) Name() string { return "graph" }

func mapGraphResults(results []repositories.GraphResult, limit int) []services.RetrieverResult {
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	out := make([]services.RetrieverResult, limit)
	for i := 0; i < limit; i++ {
		r := results[i]
		out[i] = services.RetrieverResult{
			ChunkID:  r.ChunkID,
			Content:  fmt.Sprintf("%s (depth: %d)", r.Name, r.Depth),
			FilePath: r.FilePath,
			Score:    1.0 / float32(r.Depth+1),
			Source:   "graph",
		}
	}
	return out
}

const keywordExtractionPrompt = `Extract the key technical search terms from this code search query.
Return ONLY the meaningful identifiers, function names, class names, module names, or technical terms — space-separated, lowercase.
Remove all natural language filler (how, does, what, work, etc).
If the query IS a function/class name already, return it as-is.

Query: %s
Keywords:`

// extractGraphKeywords calls OpenAI to extract meaningful search terms from a natural language query.
func extractGraphKeywords(ctx context.Context, cfg config.OpenAIConfig, query string) (string, error) {
	type responsesRequest struct {
		Model     string `json:"model"`
		Input     string `json:"input"`
		MaxTokens int    `json:"max_output_tokens"`
	}
	type outputContent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type outputItem struct {
		Type    string          `json:"type"`
		Content []outputContent `json:"content"`
	}
	type responsesResponse struct {
		Output []outputItem `json:"output"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	reqBody := responsesRequest{
		Model:     cfg.Model(),
		Input:     fmt.Sprintf(keywordExtractionPrompt, query),
		MaxTokens: 50,
	}

	client := newRestyClient(cfg.APIKey())
	var result responsesResponse

	resp, err := client.R().
		SetContext(ctx).
		SetBody(reqBody).
		SetResult(&result).
		Post(cfg.BaseURL() + "/responses")

	if err != nil {
		return "", fmt.Errorf("keyword extraction request failed: %w", err)
	}

	if resp.IsError() {
		return "", fmt.Errorf("keyword extraction API error: %s", resp.Status())
	}

	if result.Error != nil {
		return "", fmt.Errorf("keyword extraction returned error: %s", result.Error.Message)
	}

	var raw string
	for _, item := range result.Output {
		if item.Type == "message" {
			for _, c := range item.Content {
				if c.Type == "output_text" {
					raw = c.Text
					break
				}
			}
		}
	}

	return strings.TrimSpace(raw), nil
}

func (r *graphRetriever) ListFiles(ctx context.Context, filter services.SearchFilter) ([]string, error) {
	return nil, nil
}

func (r *graphRetriever) ReadFile(ctx context.Context, path string, filter services.SearchFilter) (string, error) {
	return "", fmt.Errorf("read_file not supported by graphRetriever")
}
