package impl

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// OpenAIReviewer implements services.Reviewer using the OpenAI Responses API.
type OpenAIReviewer struct {
	client       *openai.Client
	model        string
	executor     *ToolExecutor
	metrics      *services.ReviewMetrics
	prompts      *PromptProvider
	systemPrompt string
	maxTokens    int64
	temperature  float64
	maxTurns     int
	maxTurnsMsg  string
	logger       *zap.Logger
}

func NewOpenAIReviewer(
	apiKey string,
	model string,
	maxTokens int,
	maxTurns int,
	executor *ToolExecutor,
	promptProvider *PromptProvider,
	logger *zap.Logger,
) services.Reviewer {
	opts := []option.RequestOption{}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	client := openai.NewClient(opts...)

	if maxTokens <= 0 {
		maxTokens = 16384
	}
	if maxTurns <= 0 {
		maxTurns = 3
	}

	return &OpenAIReviewer{
		client:       &client,
		model:        model,
		executor:     executor,
		metrics:      services.NewReviewMetrics(),
		prompts:      promptProvider,
		systemPrompt: promptProvider.SystemReview(),
		maxTokens:    int64(maxTokens),
		maxTurns:     maxTurns,
		maxTurnsMsg:  promptProvider.MaxTurns(),
		logger:       logger.Named("openai-reviewer"),
	}
}

func (r *OpenAIReviewer) GetMetrics() *services.ReviewMetrics {
	return r.metrics
}

func (r *OpenAIReviewer) Review(ctx context.Context, input services.ReviewInput) (*services.ReviewOutput, error) {
	userMsg := r.buildInitialMessage(input)

	// Only provide tools when codebase is indexed; otherwise single-shot.
	var tools []responses.ToolUnionParam
	maxTurns := 1
	if r.executor.HasBackend() {
		tools = r.getToolDefinitions()
		maxTurns = r.maxTurns
	}

	r.executor.SetMetrics(r.metrics)

	// First request
	resp, err := r.createResponse(ctx, responses.ResponseNewParamsInputUnion{
		OfString: openai.String(userMsg),
	}, tools)
	if err != nil {
		return nil, err
	}

	// Tool loop
	for turnCount := 1; turnCount <= maxTurns; turnCount++ {
		toolCalls := r.extractToolCalls(resp)
		if len(toolCalls) == 0 {
			rawText := resp.OutputText()
			out := parseRawToReviewOutput(rawText)
			logRawIfSchemaMismatch(r.logger, rawText, out)
			return out, nil
		}

		r.logger.Info("Processing tool calls",
			zap.Int("turn", turnCount),
			zap.Int("tool_calls", len(toolCalls)))

		// Build input items: original message + all output items + tool results
		inputItems := responses.ResponseInputParam{
			responses.ResponseInputItemParamOfMessage(userMsg, responses.EasyInputMessageRoleUser),
		}
		for _, item := range resp.Output {
			if item.Type == "function_call" {
				inputItems = append(inputItems, responses.ResponseInputItemParamOfFunctionCall(
					item.Arguments, item.CallID, item.Name,
				))
			}
		}

		for _, tc := range toolCalls {
			r.logger.Info("Executing tool",
				zap.String("tool_name", tc.Name),
				zap.String("call_id", tc.CallID))

			resultStr, execErr := r.executor.Execute(ctx, tc.Name, []byte(tc.Arguments))
			if execErr != nil {
				r.logger.Error("Tool execution failed",
					zap.String("tool_name", tc.Name),
					zap.Error(execErr))
				resultStr = execErr.Error()
			}
			inputItems = append(inputItems,
				responses.ResponseInputItemParamOfFunctionCallOutput(tc.CallID, resultStr),
			)
		}

		// Force final answer on last turn
		var reqTools []responses.ToolUnionParam
		if turnCount < r.maxTurns {
			reqTools = tools
		} else {
			r.logger.Warn("Max turn limit reached, forcing final response", zap.Int("max_turns", r.maxTurns))
			inputItems = append(inputItems,
				responses.ResponseInputItemParamOfMessage(r.maxTurnsMsg, responses.EasyInputMessageRoleUser),
			)
		}

		resp, err = r.createResponse(ctx, responses.ResponseNewParamsInputUnion{
			OfInputItemList: inputItems,
		}, reqTools)
		if err != nil {
			return nil, err
		}
	}

	rawText := resp.OutputText()
	out := parseRawToReviewOutput(rawText)
	logRawIfSchemaMismatch(r.logger, rawText, out)
	return out, nil
}

func (r *OpenAIReviewer) createResponse(
	ctx context.Context,
	input responses.ResponseNewParamsInputUnion,
	tools []responses.ToolUnionParam,
) (*responses.Response, error) {
	params := responses.ResponseNewParams{
		Model:           r.model,
		Input:           input,
		Instructions:    openai.String(r.systemPrompt),
		MaxOutputTokens: openai.Int(r.maxTokens),
	}
	// Some models (e.g. gpt-5-codex) don't support temperature -- only set if non-zero
	if r.temperature > 0 {
		params.Temperature = openai.Float(r.temperature)
	}
	if len(tools) > 0 {
		params.Tools = tools
	}

	resp, err := r.client.Responses.New(ctx, params)
	if err != nil {
		return nil, err
	}

	cachedIn := int(resp.Usage.InputTokensDetails.CachedTokens)
	r.logger.Info("API response received",
		zap.String("status", string(resp.Status)),
		zap.Int("input_tokens", int(resp.Usage.InputTokens)),
		zap.Int("cached_input_tokens", cachedIn),
		zap.Int("output_tokens", int(resp.Usage.OutputTokens)))

	// OpenAI Responses caching is implicit: prefix-stable >=1024 tokens hit cache
	// automatically. Report cached_input as cache_read so cost math is accurate.
	r.metrics.RecordTokens(int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens), 0, cachedIn)

	if resp.Status == responses.ResponseStatusFailed {
		return nil, fmt.Errorf("response failed: %s", resp.Error.Message)
	}

	return resp, nil
}

type functionCall struct {
	CallID    string
	Name      string
	Arguments string
}

func (r *OpenAIReviewer) extractToolCalls(resp *responses.Response) []functionCall {
	var calls []functionCall
	for _, item := range resp.Output {
		if item.Type == "function_call" {
			calls = append(calls, functionCall{
				CallID:    item.CallID,
				Name:      item.Name,
				Arguments: item.Arguments,
			})
		}
	}
	return calls
}

func (r *OpenAIReviewer) buildInitialMessage(input services.ReviewInput) string {
	hasTools := r.executor.HasBackend()

	// Build per-file diff content
	fileDiff := buildFileDiffMessage(input.Files)
	if fileDiff == "" {
		fileDiff = input.Diff
	}

	var msg string
	if len(fileDiff) > maxMessageDiffChars {
		fileList := "\n\nFiles changed in this PR:\n"
		for _, f := range input.Files {
			fileList += fmt.Sprintf("- %s (%s, +%d -%d)\n", f.Filename, f.Status, f.Additions, f.Deletions)
		}
		toolHint := "Review based on the visible diff only."
		if hasTools {
			toolHint = "You MUST use search_codebase, get_function_context, and get_blast_radius tools to understand the full impact before writing your review."
		}
		msg = fmt.Sprintf(
			"<user_description>%s</user_description>\n\nThe diff is large (%d chars). Below is the first %d chars. %s%s"+
				"\n\nPartial diff:\n%s",
			input.Description,
			len(fileDiff),
			maxMessageDiffChars,
			toolHint,
			fileList,
			fileDiff[:maxMessageDiffChars],
		)
	} else if hasTools {
		msg = fmt.Sprintf("<user_description>%s</user_description>\n\n<user_diff>\n%s</user_diff>\n\n"+
			"Remember: use search_codebase and get_blast_radius tools to check how these changes affect the rest of the codebase before finalizing your review.",
			input.Description, fileDiff)
	} else {
		msg = fmt.Sprintf("<user_description>%s</user_description>\n\n<user_diff>\n%s</user_diff>",
			input.Description, fileDiff)
	}

	if len(input.FileContents) > 0 {
		msg += formatFileContents(input.FileContents)
	}

	if input.GraphContext != "" {
		msg += "\n\n" + input.GraphContext
	}

	if len(input.CalleeBodies) > 0 {
		msg += formatCalleeBodies(input.CalleeBodies)
	}

	if input.SimilarCode != "" {
		msg += input.SimilarCode
	}

	if len(input.ExistingComments) > 0 {
		msg += formatExistingComments(input.ExistingComments)
	}
	return msg
}

func formatExistingComments(comments []services.ExistingComment) string {
	if len(comments) == 0 {
		return ""
	}
	msg := "\n\n<existing_unresolved_comments>\n"
	for _, c := range comments {
		msg += fmt.Sprintf("- [id:%s] [%s] %s:%d — %s\n", c.ID, c.Severity, c.FilePath, c.LineNumber, c.Body)
	}
	msg += "</existing_unresolved_comments>"
	return msg
}

func (r *OpenAIReviewer) getToolDefinitions() []responses.ToolUnionParam {
	return []responses.ToolUnionParam{
		{
			OfFunction: &responses.FunctionToolParam{
				Name:        "search_codebase",
				Description: openai.String("Search the codebase for specific code patterns, syntax, or functionality using semantic search."),
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "The search query, e.g., 'database connection logic' or 'auth middleware'.",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			OfFunction: &responses.FunctionToolParam{
				Name:        "get_function_context",
				Description: openai.String("Find and return the full source code and context of a specific function or method by name."),
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"function_name": map[string]any{
							"type":        "string",
							"description": "The exact name of the function or method to find.",
						},
					},
					"required": []string{"function_name"},
				},
			},
		},
		{
			OfFunction: &responses.FunctionToolParam{
				Name:        "get_file_tree",
				Description: openai.String("Get the directory structure of the repository to understand the project layout."),
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		{
			OfFunction: &responses.FunctionToolParam{
				Name:        "get_blast_radius",
				Description: openai.String("Find all functions that directly or transitively call a given function (upstream impact)."),
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"function_name": map[string]any{
							"type":        "string",
							"description": "The name of the function to find upstream callers for.",
						},
						"file_path": map[string]any{
							"type":        "string",
							"description": "Optional file path to disambiguate when multiple functions share the same name.",
						},
					},
					"required": []string{"function_name"},
				},
			},
		},
		{
			OfFunction: &responses.FunctionToolParam{
				Name:        "get_dependencies",
				Description: openai.String("Find all functions that a given function calls (downstream dependencies)."),
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"function_name": map[string]any{
							"type":        "string",
							"description": "The name of the function to find downstream dependencies for.",
						},
						"file_path": map[string]any{
							"type":        "string",
							"description": "Optional file path to disambiguate when multiple functions share the same name.",
						},
					},
					"required": []string{"function_name"},
				},
			},
		},
		{
			OfFunction: &responses.FunctionToolParam{
				Name:        "read_file",
				Description: openai.String("Read the full content of a file from the indexed codebase."),
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "The file path relative to the repository root, e.g. 'pkg/operations/interfaces.go'.",
						},
					},
					"required": []string{"path"},
				},
			},
		},
	}
}
