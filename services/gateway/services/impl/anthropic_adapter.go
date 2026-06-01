package impl

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

// AnthropicAdapter translates Chat Completions format to/from Anthropic Messages API.
type AnthropicAdapter struct {
	baseURL          string
	version          string
	defaultMaxTokens int
	apiKey           string
	logger           *zap.Logger
}

func NewAnthropicAdapter(baseURL, version string, defaultMaxTokens int, apiKey string, logger *zap.Logger) *AnthropicAdapter {
	if defaultMaxTokens == 0 {
		defaultMaxTokens = 8192
	}
	return &AnthropicAdapter{
		baseURL:          strings.TrimRight(baseURL, "/"),
		version:          version,
		defaultMaxTokens: defaultMaxTokens,
		apiKey:           apiKey,
		logger:           logger.Named("gateway.anthropic"),
	}
}

func (a *AnthropicAdapter) ConfiguredAPIKey() string {
	return a.apiKey
}

func (a *AnthropicAdapter) Name() string {
	return "anthropic"
}

func (a *AnthropicAdapter) MatchesModel(model string) bool {
	return strings.HasPrefix(model, "claude-")
}

func (a *AnthropicAdapter) TranslateRequest(req *dto.ChatCompletionRequest, apiKey string) (string, map[string]string, []byte, error) {
	anthropicReq := dto.AnthropicRequest{
		Model:  req.Model,
		Stream: true,
	}

	if system := extractSystemMessages(req.Messages); system != nil {
		anthropicReq.System = system
	}

	if req.CacheControl != nil {
		anthropicReq.CacheControl = toAnthropicCacheControl(req.CacheControl)
	}

	// Translate messages
	msgs, err := translateMessages(req.Messages)
	if err != nil {
		return "", nil, nil, fmt.Errorf("translate messages: %w", err)
	}
	anthropicReq.Messages = msgs

	// Max tokens
	if req.MaxCompletionTokens != nil {
		anthropicReq.MaxTokens = *req.MaxCompletionTokens
	} else {
		anthropicReq.MaxTokens = a.defaultMaxTokens
	}

	// Temperature
	if req.Temperature != nil {
		temp := *req.Temperature
		anthropicReq.Temperature = &temp
	}

	// Stop sequences
	if len(req.Stop) > 0 {
		stopSeqs, err := parseStopSequences(req.Stop)
		if err == nil && len(stopSeqs) > 0 {
			anthropicReq.StopSequences = stopSeqs
		}
	}

	// Tools
	if len(req.Tools) > 0 {
		anthropicReq.Tools = translateTools(req.Tools)
	}

	// Tool choice
	if len(req.ToolChoice) > 0 {
		tc, err := translateToolChoice(req.ToolChoice)
		if err == nil && tc != nil {
			if req.ParallelToolCalls != nil && !*req.ParallelToolCalls {
				tc.DisableParallelToolUse = true
			}
			anthropicReq.ToolChoice = tc
		}
	}

	// Thinking
	if req.Thinking != nil && req.Thinking.BudgetTokens > 0 {
		anthropicReq.Thinking = &dto.AnthropicThinkingConfig{
			Type:         "enabled",
			BudgetTokens: req.Thinking.BudgetTokens,
		}
		// Anthropic requires temperature=1 with thinking
		temp := 1.0
		anthropicReq.Temperature = &temp
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return "", nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := a.baseURL + "/v1/messages"
	headers := map[string]string{
		"Content-Type":      "application/json",
		"x-api-key":         apiKey,
		"anthropic-version": a.version,
	}

	return url, headers, body, nil
}

// extractSystemMessages returns nil, a plain string, or []AnthropicSystemBlock
// (block-array form used when any block has cache_control).
func extractSystemMessages(messages []dto.Message) interface{} {
	var blocks []dto.AnthropicSystemBlock
	var anyHasCacheControl bool

	for _, msg := range messages {
		if msg.Role != "system" {
			continue
		}
		text, contentBlocks, _ := dto.ParseMessageContent(msg.Content)
		if len(contentBlocks) > 0 {
			for _, b := range contentBlocks {
				if b.Type != "text" || b.Text == "" {
					continue
				}
				block := dto.AnthropicSystemBlock{Type: "text", Text: b.Text}
				if b.CacheControl != nil {
					block.CacheControl = toAnthropicCacheControl(b.CacheControl)
					anyHasCacheControl = true
				}
				blocks = append(blocks, block)
			}
		} else if text != "" {
			blocks = append(blocks, dto.AnthropicSystemBlock{Type: "text", Text: text})
		}
	}

	if len(blocks) == 0 {
		return nil
	}
	if anyHasCacheControl {
		return blocks
	}
	// No cache markers — fold back to plain string for back-compat.
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, b.Text)
	}
	return strings.Join(parts, "\n\n")
}

func toAnthropicCacheControl(src *dto.CacheControl) *dto.AnthropicCacheControl {
	if src == nil {
		return nil
	}
	return &dto.AnthropicCacheControl{Type: src.Type, TTL: src.TTL}
}

func translateMessages(messages []dto.Message) ([]dto.AnthropicMessage, error) {
	var result []dto.AnthropicMessage
	var pendingToolResults []dto.AnthropicContentBlock

	for _, msg := range messages {
		if msg.Role == "system" {
			continue
		}

		if msg.Role == "tool" {
			text, _, _ := dto.ParseMessageContent(msg.Content)
			block := dto.AnthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: sanitizeToolID(msg.ToolCallID),
			}
			if text != "" {
				contentBytes, err := json.Marshal(text)
				if err != nil {
					return nil, fmt.Errorf("marshal tool result content: %w", err)
				}
				block.Content = contentBytes
			}
			pendingToolResults = append(pendingToolResults, block)
			continue
		}

		// Flush pending tool results as a user message before non-tool messages
		if len(pendingToolResults) > 0 {
			var err error
			result, err = flushToolResults(result, pendingToolResults)
			if err != nil {
				return nil, err
			}
			pendingToolResults = nil
		}

		switch msg.Role {
		case "user":
			amsg, err := translateUserMessage(msg)
			if err != nil {
				return nil, err
			}
			result = appendWithAdjacency(result, amsg)

		case "assistant":
			amsg, err := translateAssistantMessage(msg)
			if err != nil {
				return nil, err
			}
			result = appendWithAdjacency(result, amsg)
		}
	}

	// Flush any remaining tool results
	if len(pendingToolResults) > 0 {
		var err error
		result, err = flushToolResults(result, pendingToolResults)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func flushToolResults(result []dto.AnthropicMessage, toolResults []dto.AnthropicContentBlock) ([]dto.AnthropicMessage, error) {
	contentBytes, err := json.Marshal(toolResults)
	if err != nil {
		return nil, fmt.Errorf("marshal tool results: %w", err)
	}
	msg := dto.AnthropicMessage{
		Role:    "user",
		Content: contentBytes,
	}
	return appendWithAdjacency(result, msg), nil
}

func translateUserMessage(msg dto.Message) (dto.AnthropicMessage, error) {
	text, blocks, err := dto.ParseMessageContent(msg.Content)
	if err != nil {
		return dto.AnthropicMessage{}, err
	}

	if len(blocks) > 0 {
		var contentBlocks []dto.AnthropicContentBlock
		for _, block := range blocks {
			switch block.Type {
			case "text":
				contentBlocks = append(contentBlocks, dto.AnthropicContentBlock{
					Type:         "text",
					Text:         block.Text,
					CacheControl: toAnthropicCacheControl(block.CacheControl),
				})
			case "image_url":
				if block.ImageURL != nil {
					imgBlock, err := translateImageBlock(block.ImageURL.URL)
					if err != nil {
						return dto.AnthropicMessage{}, err
					}
					imgBlock.CacheControl = toAnthropicCacheControl(block.CacheControl)
					contentBlocks = append(contentBlocks, imgBlock)
				}
			}
		}
		contentBytes, err := json.Marshal(contentBlocks)
		if err != nil {
			return dto.AnthropicMessage{}, fmt.Errorf("marshal user content blocks: %w", err)
		}
		return dto.AnthropicMessage{Role: "user", Content: contentBytes}, nil
	}

	contentBytes, err := json.Marshal(text)
	if err != nil {
		return dto.AnthropicMessage{}, fmt.Errorf("marshal user text content: %w", err)
	}
	return dto.AnthropicMessage{Role: "user", Content: contentBytes}, nil
}

func translateImageBlock(url string) (dto.AnthropicContentBlock, error) {
	mediaType, data, err := parseDataURI(url)
	if err != nil {
		return dto.AnthropicContentBlock{}, fmt.Errorf("parse image data URI: %w", err)
	}
	return dto.AnthropicContentBlock{
		Type: "image",
		Source: &dto.AnthropicImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      data,
		},
	}, nil
}

var toolIDPattern = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitizeToolID(id string) string {
	return toolIDPattern.ReplaceAllString(id, "_")
}

func translateAssistantMessage(msg dto.Message) (dto.AnthropicMessage, error) {
	var contentBlocks []dto.AnthropicContentBlock

	// Add thinking blocks if present
	if len(msg.Thinking) > 0 {
		var thinking string
		if err := json.Unmarshal(msg.Thinking, &thinking); err == nil && thinking != "" {
			contentBlocks = append(contentBlocks, dto.AnthropicContentBlock{
				Type:     "thinking",
				Thinking: thinking,
			})
		}
	}

	// Add text content
	text, _, _ := dto.ParseMessageContent(msg.Content)
	if text != "" {
		contentBlocks = append(contentBlocks, dto.AnthropicContentBlock{
			Type: "text",
			Text: text,
		})
	}

	// Convert tool calls to tool_use blocks
	for _, tc := range msg.ToolCalls {
		var input json.RawMessage
		if tc.Function.Arguments != "" && json.Valid([]byte(tc.Function.Arguments)) {
			input = json.RawMessage(tc.Function.Arguments)
		} else {
			input = json.RawMessage("{}")
		}
		contentBlocks = append(contentBlocks, dto.AnthropicContentBlock{
			Type:  "tool_use",
			ID:    sanitizeToolID(tc.ID),
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	if len(contentBlocks) == 0 {
		contentBytes, err := json.Marshal("")
		if err != nil {
			return dto.AnthropicMessage{}, fmt.Errorf("marshal empty assistant content: %w", err)
		}
		return dto.AnthropicMessage{Role: "assistant", Content: contentBytes}, nil
	}

	contentBytes, err := json.Marshal(contentBlocks)
	if err != nil {
		return dto.AnthropicMessage{}, fmt.Errorf("marshal assistant content blocks: %w", err)
	}
	return dto.AnthropicMessage{Role: "assistant", Content: contentBytes}, nil
}

// appendWithAdjacency merges consecutive same-role messages (Anthropic requires alternation).
func appendWithAdjacency(result []dto.AnthropicMessage, msg dto.AnthropicMessage) []dto.AnthropicMessage {
	if len(result) == 0 || result[len(result)-1].Role != msg.Role {
		return append(result, msg)
	}

	// Merge content arrays
	last := &result[len(result)-1]
	lastBlocks := parseContentBlocks(last.Content)
	newBlocks := parseContentBlocks(msg.Content)
	merged := append(lastBlocks, newBlocks...)
	mergedBytes, err := json.Marshal(merged)
	if err != nil {
		// Best effort: append as separate message rather than losing data
		return append(result, msg)
	}
	last.Content = mergedBytes
	return result
}

func parseContentBlocks(raw json.RawMessage) []dto.AnthropicContentBlock {
	var blocks []dto.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		// It might be a string
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && s != "" {
			return []dto.AnthropicContentBlock{{Type: "text", Text: s}}
		}
		return nil
	}
	return blocks
}

func translateTools(tools []dto.Tool) []dto.AnthropicTool {
	var result []dto.AnthropicTool
	for _, t := range tools {
		result = append(result, dto.AnthropicTool{
			Name:         t.Function.Name,
			Description:  t.Function.Description,
			InputSchema:  t.Function.Parameters,
			CacheControl: toAnthropicCacheControl(t.CacheControl),
		})
	}
	return result
}

func translateToolChoice(raw json.RawMessage) (*dto.AnthropicToolChoice, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			return &dto.AnthropicToolChoice{Type: "auto"}, nil
		case "required":
			return &dto.AnthropicToolChoice{Type: "any"}, nil
		case "none":
			return nil, nil
		default:
			return &dto.AnthropicToolChoice{Type: "auto"}, nil
		}
	}

	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Function.Name != "" {
		return &dto.AnthropicToolChoice{
			Type: "tool",
			Name: obj.Function.Name,
		}, nil
	}

	return &dto.AnthropicToolChoice{Type: "auto"}, nil
}

func parseStopSequences(raw json.RawMessage) ([]string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []string{s}, nil
	}

	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

func parseDataURI(uri string) (mediaType, data string, err error) {
	// Format: data:{media_type};base64,{data}
	if !strings.HasPrefix(uri, "data:") {
		return "", "", fmt.Errorf("not a data URI")
	}
	rest := strings.TrimPrefix(uri, "data:")
	parts := strings.SplitN(rest, ";base64,", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid data URI format")
	}
	// Validate base64
	if _, err := base64.StdEncoding.DecodeString(parts[1]); err != nil {
		// Try raw base64 (no padding)
		if _, err := base64.RawStdEncoding.DecodeString(parts[1]); err != nil {
			return "", "", fmt.Errorf("invalid base64 data: %w", err)
		}
	}
	return parts[0], parts[1], nil
}
