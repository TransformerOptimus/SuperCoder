package dto

import "encoding/json"

// AnthropicRequest is the outbound request to Anthropic's Messages API.
type AnthropicRequest struct {
	Model         string                   `json:"model"`
	System        interface{}              `json:"system,omitempty"` // string or []AnthropicSystemBlock
	Messages      []AnthropicMessage       `json:"messages"`
	Tools         []AnthropicTool          `json:"tools,omitempty"`
	Stream        bool                     `json:"stream"`
	MaxTokens     int                      `json:"max_tokens"`
	Temperature   *float64                 `json:"temperature,omitempty"`
	StopSequences []string                 `json:"stop_sequences,omitempty"`
	ToolChoice    *AnthropicToolChoice     `json:"tool_choice,omitempty"`
	Thinking      *AnthropicThinkingConfig `json:"thinking,omitempty"`
	CacheControl  *AnthropicCacheControl   `json:"cache_control,omitempty"` // top-level automatic caching
}

type AnthropicCacheControl struct {
	Type string `json:"type"`          // "ephemeral"
	TTL  string `json:"ttl,omitempty"` // "5m" or "1h"
}

// AnthropicSystemBlock is the array form of the system field, needed for per-block cache_control.
type AnthropicSystemBlock struct {
	Type         string                 `json:"type"` // "text"
	Text         string                 `json:"text"`
	CacheControl *AnthropicCacheControl `json:"cache_control,omitempty"`
}

type AnthropicThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type AnthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// AnthropicContentBlock is a union type for content blocks in Anthropic messages.
type AnthropicContentBlock struct {
	Type string `json:"type"`

	// text block
	Text string `json:"text,omitempty"`

	// tool_use block
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result block
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`

	// thinking block
	Thinking string `json:"thinking,omitempty"`

	// image block
	Source *AnthropicImageSource `json:"source,omitempty"`

	CacheControl *AnthropicCacheControl `json:"cache_control,omitempty"`
}

type AnthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type AnthropicTool struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  json.RawMessage        `json:"input_schema"`
	CacheControl *AnthropicCacheControl `json:"cache_control,omitempty"`
}

type AnthropicToolChoice struct {
	Type                   string `json:"type"`
	Name                   string `json:"name,omitempty"`
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use,omitempty"`
}

// Anthropic SSE event data types

type MessageStartData struct {
	Type    string              `json:"type"`
	Message MessageStartMessage `json:"message"`
}

type MessageStartMessage struct {
	ID    string             `json:"id"`
	Model string             `json:"model"`
	Usage *MessageStartUsage `json:"usage,omitempty"`
}

type MessageStartUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

type ContentBlockStartData struct {
	Index        int                    `json:"index"`
	ContentBlock ContentBlockStartBlock `json:"content_block"`
}

type ContentBlockStartBlock struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
}

type ContentBlockDeltaData struct {
	Index int                    `json:"index"`
	Delta ContentBlockDeltaDelta `json:"delta"`
}

type ContentBlockDeltaDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
}

type ContentBlockStopData struct {
	Index int `json:"index"`
}

type MessageDeltaData struct {
	Delta MessageDeltaDelta  `json:"delta"`
	Usage *MessageDeltaUsage `json:"usage,omitempty"`
}

type MessageDeltaDelta struct {
	StopReason string `json:"stop_reason,omitempty"`
}

type MessageDeltaUsage struct {
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

type AnthropicErrorData struct {
	Type  string               `json:"type"`
	Error AnthropicErrorDetail `json:"error"`
}

type AnthropicErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
