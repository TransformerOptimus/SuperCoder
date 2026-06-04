package impl

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// AnthropicReviewer implements services.Reviewer using the Anthropic Claude API.
type AnthropicReviewer struct {
	client       *anthropic.Client
	model        string
	executor     *ToolExecutor
	metrics      *services.ReviewMetrics
	prompts      *PromptProvider
	systemPrompt string
	maxTokens    int64
	maxTurns     int
	maxTurnsMsg  string
	logger       *zap.Logger
}

func NewAnthropicReviewer(
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
	client := anthropic.NewClient(opts...)

	if maxTokens <= 0 {
		maxTokens = 16384
	}
	if maxTurns <= 0 {
		maxTurns = 3
	}

	return &AnthropicReviewer{
		client:       &client,
		model:        model,
		executor:     executor,
		metrics:      services.NewReviewMetrics(),
		prompts:      promptProvider,
		systemPrompt: promptProvider.SystemReview(),
		maxTokens:    int64(maxTokens),
		maxTurns:     maxTurns,
		maxTurnsMsg:  promptProvider.MaxTurns(),
		logger:       logger.Named("reviewer"),
	}
}

func (r *AnthropicReviewer) GetMetrics() *services.ReviewMetrics {
	return r.metrics
}

func (r *AnthropicReviewer) streamMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	stream := r.client.Messages.NewStreaming(ctx, params)

	accumulatedMessage := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		if err := accumulatedMessage.Accumulate(event); err != nil {
			r.logger.Error("failed to accumulate stream event", zap.Error(err))
			continue
		}
	}
	if stream.Err() != nil {
		return nil, stream.Err()
	}
	return &accumulatedMessage, nil
}

func (r *AnthropicReviewer) Review(ctx context.Context, input services.ReviewInput) (*services.ReviewOutput, error) {
	userMsg := r.buildInitialMessage(input)

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userMsg)),
	}

	r.executor.SetMetrics(r.metrics)

	// Cache the system prompt across all reviews (identical every time).
	cacheControl := anthropic.NewCacheControlEphemeralParam()
	cacheControl.TTL = anthropic.CacheControlEphemeralTTLTTL1h

	systemBlocks := []anthropic.TextBlockParam{{
		Text:         r.systemPrompt,
		CacheControl: cacheControl,
	}}

	// Only provide tools when codebase is indexed; otherwise single-shot.
	var toolDefs []anthropic.ToolUnionParam
	maxTurns := 1
	if r.executor.HasBackend() {
		toolDefs = GetToolDefinitions()
		// Cache the last tool definition so the full system+tools prefix is cached.
		toolDefs[len(toolDefs)-1].OfTool.CacheControl = cacheControl
		maxTurns = r.maxTurns
	}

	turnCount := 0
	for {
		turnCount++
		if turnCount > maxTurns {
			r.logger.Warn("Max turn limit reached, forcing final response", zap.Int("max_turns", r.maxTurns))
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(r.maxTurnsMsg),
			))
			resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
				Model:     anthropic.Model(r.model),
				MaxTokens: r.maxTokens,
				System:    systemBlocks,
				Messages:  messages,
			})
			if err != nil {
				return nil, err
			}
			r.metrics.RecordTokens(
				int(resp.Usage.InputTokens),
				int(resp.Usage.OutputTokens),
				int(resp.Usage.CacheCreationInputTokens),
				int(resp.Usage.CacheReadInputTokens),
			)
			rawText := r.extractText(resp)
			out := parseRawToReviewOutput(rawText)
			logRawIfSchemaMismatch(r.logger, rawText, out)
			return out, nil
		}

		r.logger.Info("API request",
			zap.Int("turn", turnCount),
			zap.Int("message_count", len(messages)),
			zap.Bool("tools_enabled", len(toolDefs) > 0))

		resp, err := r.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(r.model),
			MaxTokens: r.maxTokens,
			System:    systemBlocks,
			Tools:     toolDefs,
			Messages:  messages,
		})
		if err != nil {
			r.logger.Error("API request failed", zap.Int("turn", turnCount), zap.Error(err))
			return nil, err
		}

		r.metrics.RecordTokens(
			int(resp.Usage.InputTokens),
			int(resp.Usage.OutputTokens),
			int(resp.Usage.CacheCreationInputTokens),
			int(resp.Usage.CacheReadInputTokens),
		)

		r.logger.Info("API response received",
			zap.Int("turn", turnCount),
			zap.String("stop_reason", string(resp.StopReason)),
			zap.Int("input_tokens", int(resp.Usage.InputTokens)),
			zap.Int("output_tokens", int(resp.Usage.OutputTokens)),
			zap.Int64("cache_creation_tokens", resp.Usage.CacheCreationInputTokens),
			zap.Int64("cache_read_tokens", resp.Usage.CacheReadInputTokens))

		// End turn — return text (single-shot for non-truncated diffs)
		if resp.StopReason == anthropic.StopReasonEndTurn {
			rawText := r.extractText(resp)
			out := parseRawToReviewOutput(rawText)
			logRawIfSchemaMismatch(r.logger, rawText, out)
			return out, nil
		}

		// Tool use — execute and continue (only for truncated diffs)
		if resp.StopReason == anthropic.StopReasonToolUse {
			messages = append(messages, resp.ToParam())

			var toolResultBlocks []anthropic.ContentBlockParamUnion
			for _, content := range resp.Content {
				if content.Type == "tool_use" {
					toolUse := content.AsToolUse()
					r.logger.Info("Executing tool",
						zap.String("tool_name", toolUse.Name),
						zap.String("tool_id", toolUse.ID))

					resultStr, execErr := r.executor.Execute(ctx, toolUse.Name, []byte(toolUse.Input))

					isErr := false
					if execErr != nil {
						r.logger.Error("Tool execution failed",
							zap.String("tool_name", toolUse.Name),
							zap.Error(execErr))
						resultStr = execErr.Error()
						isErr = true
					} else {
						r.logger.Info("Tool execution completed",
							zap.String("tool_name", toolUse.Name),
							zap.Int("result_len", len(resultStr)))
					}
					toolResultBlocks = append(toolResultBlocks, anthropic.NewToolResultBlock(toolUse.ID, resultStr, isErr))
				}
			}

			if len(toolResultBlocks) > 0 {
				// Set cache breakpoint on the last tool result so the entire
				// conversation prefix (system + tools + prior turns) is cached
				// for the next turn within this review.
				last := &toolResultBlocks[len(toolResultBlocks)-1]
				last.OfToolResult.CacheControl = cacheControl
				messages = append(messages, anthropic.NewUserMessage(toolResultBlocks...))
			}
			continue
		}

		// Unexpected stop reason
		rawText := r.extractText(resp)
		out := parseRawToReviewOutput(rawText)
		logRawIfSchemaMismatch(r.logger, rawText, out)
		return out, nil
	}
}

func (r *AnthropicReviewer) extractText(resp *anthropic.Message) string {
	for _, content := range resp.Content {
		if content.Type == "text" {
			return content.AsText().Text
		}
	}
	return "Review complete but no text output generated."
}

// maxMessageDiffChars is the limit for inline diff in the initial message.
// Diffs larger than this are truncated, encouraging the model to use codebase tools
// for full file content and blast radius analysis.
const maxMessageDiffChars = 10_000

func (r *AnthropicReviewer) buildInitialMessage(input services.ReviewInput) string {
	hasTools := r.executor.HasBackend()

	// Build per-file diff content
	fileDiff := buildFileDiffMessage(input.Files)
	if fileDiff == "" {
		// Fallback to reconstructed diff if no per-file patches
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

	// Order: file contents → graph context → similar code → existing comments.
	// Static/shared content first (cache-friendly for Anthropic),
	// per-chunk dynamic content last.
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
