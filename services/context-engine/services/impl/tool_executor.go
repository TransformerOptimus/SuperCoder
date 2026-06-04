package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

const maxToolResultChars = 2000

type ToolExecutor struct {
	retriever      services.RetrieverService
	graph          repositories.GraphRepository
	repoPath       string
	collectionName string
	filter         services.SearchFilter
	metrics        *services.ReviewMetrics
	logger         *zap.Logger
}

func NewToolExecutor(
	ret services.RetrieverService,
	graph repositories.GraphRepository,
	repoPath string,
	collectionName string,
	filter services.SearchFilter,
	logger *zap.Logger,
) *ToolExecutor {
	return &ToolExecutor{
		retriever:      ret,
		graph:          graph,
		repoPath:       repoPath,
		collectionName: collectionName,
		filter:         filter,
		logger:         logger.Named("tools"),
	}
}

func (e *ToolExecutor) HasBackend() bool {
	return e.retriever != nil
}

func (e *ToolExecutor) SetMetrics(m *services.ReviewMetrics) {
	e.metrics = m
}

func GetToolDefinitions() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name:        "search_codebase",
				Description: anthropic.String("Search the codebase for specific code patterns, syntax, or functionality using semantic search."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "The search query, e.g., 'database connection logic' or 'auth middleware'.",
						},
					},
					Required: []string{"query"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "get_function_context",
				Description: anthropic.String("Find and return the full source code and context of a specific function or method by name."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"function_name": map[string]interface{}{
							"type":        "string",
							"description": "The exact name of the function or method to find.",
						},
					},
					Required: []string{"function_name"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "get_file_tree",
				Description: anthropic.String("Get the directory structure of the repository to understand the project layout."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "get_blast_radius",
				Description: anthropic.String("Find all functions that directly or transitively call a given function (upstream impact)."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"function_name": map[string]interface{}{
							"type":        "string",
							"description": "The name of the function to find upstream callers for.",
						},
						"file_path": map[string]interface{}{
							"type":        "string",
							"description": "Optional file path to disambiguate when multiple functions share the same name.",
						},
					},
					Required: []string{"function_name"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "get_dependencies",
				Description: anthropic.String("Find all functions that a given function calls (downstream dependencies)."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"function_name": map[string]interface{}{
							"type":        "string",
							"description": "The name of the function to find downstream dependencies for.",
						},
						"file_path": map[string]interface{}{
							"type":        "string",
							"description": "Optional file path to disambiguate when multiple functions share the same name.",
						},
					},
					Required: []string{"function_name"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "read_file",
				Description: anthropic.String("Read the full content of a file from the indexed codebase."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "The file path relative to the repository root, e.g. 'pkg/operations/interfaces.go'.",
						},
					},
					Required: []string{"path"},
				},
			},
		},
	}
}

func (e *ToolExecutor) Execute(ctx context.Context, toolName string, input []byte) (string, error) {
	e.logger.Info("Tool execution requested", zap.String("tool", toolName))

	switch toolName {
	case "search_codebase":
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		return e.searchCodebase(ctx, args.Query)
	case "get_function_context":
		var args struct {
			FunctionName string `json:"function_name"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		return e.getFunctionContext(ctx, args.FunctionName)
	case "get_file_tree":
		return e.getFileTree(ctx)
	case "read_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		return e.readFile(ctx, args.Path)
	case "get_blast_radius":
		var args struct {
			FunctionName string `json:"function_name"`
			FilePath     string `json:"file_path"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		return e.getBlastRadius(ctx, args.FunctionName, args.FilePath)
	case "get_dependencies":
		var args struct {
			FunctionName string `json:"function_name"`
			FilePath     string `json:"file_path"`
		}
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
		return e.getDependencies(ctx, args.FunctionName, args.FilePath)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (e *ToolExecutor) searchCodebase(ctx context.Context, query string) (string, error) {
	if e.retriever == nil {
		return "Codebase search is not available — the repository has not been indexed.", nil
	}

	timer := services.NewTimer()

	results, err := e.retriever.Search(ctx, query, 3, e.filter)
	if err != nil {
		return "", err
	}

	elapsed := timer.Elapsed()
	if e.metrics != nil {
		e.metrics.RecordSearch(len(results), elapsed)
		e.metrics.RecordToolExecution("search_codebase", elapsed)
	}

	if len(results) == 0 {
		return "No results found.", nil
	}

	var out string
	for i, res := range results {
		content := truncate(res.Content, maxToolResultChars)
		out += fmt.Sprintf("Result %d (File: %s, Score: %.2f, Source: %s):\n%s\n\n",
			i+1, res.FilePath, res.Score, res.Source, content)
	}
	return out, nil
}

func (e *ToolExecutor) getFunctionContext(ctx context.Context, functionName string) (string, error) {
	if e.retriever == nil {
		return "Function search is not available — the repository has not been indexed.", nil
	}

	timer := services.NewTimer()

	results, err := e.retriever.FindFunction(ctx, functionName, e.filter)
	if err != nil {
		return "", err
	}

	elapsed := timer.Elapsed()
	if e.metrics != nil {
		e.metrics.RecordSearch(len(results), elapsed)
		e.metrics.RecordToolExecution("get_function_context", elapsed)
	}

	if len(results) == 0 {
		return "Function not found.", nil
	}

	res := results[0]
	content := truncate(res.Content, maxToolResultChars)
	return fmt.Sprintf("Found in %s (score: %.2f, source: %s):\n%s", res.FilePath, res.Score, res.Source, content), nil
}

func (e *ToolExecutor) getFileTree(ctx context.Context) (string, error) {
	if e.retriever == nil {
		return "File tree is not available — the repository has not been indexed.", nil
	}

	timer := services.NewTimer()

	files, err := e.retriever.ListFiles(ctx, e.filter)
	if err != nil {
		return "", fmt.Errorf("failed to list files: %w", err)
	}

	// Deduplicate and sort file paths
	seen := make(map[string]bool)
	var paths []string
	for _, f := range files {
		if !seen[f] {
			seen[f] = true
			paths = append(paths, f)
		}
	}
	sort.Strings(paths)

	// Build a compact tree representation
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString(p)
		sb.WriteByte('\n')
	}

	if e.metrics != nil {
		e.metrics.RecordToolExecution("get_file_tree", timer.Elapsed())
	}

	return truncate(sb.String(), maxToolResultChars*2), nil
}

func (e *ToolExecutor) readFile(ctx context.Context, path string) (string, error) {
	if e.retriever == nil {
		return "File reading is not available — the repository has not been indexed.", nil
	}

	timer := services.NewTimer()

	content, err := e.retriever.ReadFile(ctx, path, e.filter)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}

	if e.metrics != nil {
		e.metrics.RecordToolExecution("read_file", timer.Elapsed())
	}

	return truncate(content, maxToolResultChars*3), nil
}

func (e *ToolExecutor) getBlastRadius(ctx context.Context, functionName, filePath string) (string, error) {
	timer := services.NewTimer()

	if e.graph == nil {
		return "Knowledge graph not available.", nil
	}

	results, err := e.graph.GetBlastRadius(ctx, e.collectionName, functionName, filePath, e.filter)
	if err != nil {
		return fmt.Sprintf("Error querying blast radius: %v", err), nil
	}

	elapsed := timer.Elapsed()
	if e.metrics != nil {
		e.metrics.RecordToolExecution("get_blast_radius", elapsed)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No callers found for function '%s'.", functionName), nil
	}

	var out string
	out += fmt.Sprintf("Blast radius for '%s' — %d upstream caller(s):\n\n", functionName, len(results))
	for i, r := range results {
		out += fmt.Sprintf("%d. %s (file: %s, depth: %d)\n", i+1, r.Name, r.FilePath, r.Depth)
	}
	return out, nil
}

func (e *ToolExecutor) getDependencies(ctx context.Context, functionName, filePath string) (string, error) {
	timer := services.NewTimer()

	if e.graph == nil {
		return "Knowledge graph not available.", nil
	}

	results, err := e.graph.GetDependencies(ctx, e.collectionName, functionName, filePath, e.filter)
	if err != nil {
		return fmt.Sprintf("Error querying dependencies: %v", err), nil
	}

	elapsed := timer.Elapsed()
	if e.metrics != nil {
		e.metrics.RecordToolExecution("get_dependencies", elapsed)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No dependencies found for function '%s'.", functionName), nil
	}

	var out string
	out += fmt.Sprintf("Dependencies of '%s' — %d downstream function(s):\n\n", functionName, len(results))
	for i, r := range results {
		out += fmt.Sprintf("%d. %s (file: %s, depth: %d)\n", i+1, r.Name, r.FilePath, r.Depth)
	}
	return out, nil
}

func truncate(s string, maxChars int) string {
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "\n... [truncated]"
}
