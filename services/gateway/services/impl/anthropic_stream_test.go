package impl

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

func collectChunks(t *testing.T, adapter *AnthropicAdapter, sseData string) []*dto.ChatCompletionChunk {
	t.Helper()
	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)

	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var result []*dto.ChatCompletionChunk
	for chunk := range chunks {
		result = append(result, chunk)
	}
	return result
}

func TestAnthropicStream_TextResponse(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_123","model":"claude-sonnet-4-6","usage":{"input_tokens":10}}}

event: content_block_start
data: {"index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"index":0,"delta":{"type":"text_delta","text":" world"}}

event: content_block_stop
data: {"index":0}

event: message_delta
data: {"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

event: message_stop
data: {}

`

	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)
	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var result []*dto.ChatCompletionChunk
	for chunk := range chunks {
		result = append(result, chunk)
	}

	// Should have: role chunk, "Hello", " world", finish chunk, nil (DONE)
	require.GreaterOrEqual(t, len(result), 4)

	// First chunk: role
	assert.Equal(t, "assistant", result[0].Choices[0].Delta.Role)
	assert.Equal(t, "msg_123", result[0].ID)

	// Text deltas
	assert.Equal(t, "Hello", result[1].Choices[0].Delta.Content)
	assert.Equal(t, " world", result[2].Choices[0].Delta.Content)

	// Finish chunk
	require.NotNil(t, result[3].Choices[0].FinishReason)
	assert.Equal(t, "stop", *result[3].Choices[0].FinishReason)
	require.NotNil(t, result[3].Usage)
	assert.Equal(t, 10, result[3].Usage.PromptTokens)
	assert.Equal(t, 5, result[3].Usage.CompletionTokens)

	// DONE sentinel
	assert.Nil(t, result[4])
}

func TestAnthropicStream_ToolUse(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_456","model":"claude-sonnet-4-6","usage":{"input_tokens":15}}}

event: content_block_start
data: {"index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"get_weather"}}

event: content_block_delta
data: {"index":0,"delta":{"type":"input_json_delta","partial_json":"{\"loc"}}

event: content_block_delta
data: {"index":0,"delta":{"type":"input_json_delta","partial_json":"ation\":\"NYC\"}"}}

event: content_block_stop
data: {"index":0}

event: message_delta
data: {"delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":20}}

event: message_stop
data: {}

`

	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)
	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var result []*dto.ChatCompletionChunk
	for chunk := range chunks {
		result = append(result, chunk)
	}

	require.GreaterOrEqual(t, len(result), 4)

	// Role chunk
	assert.Equal(t, "assistant", result[0].Choices[0].Delta.Role)

	// Tool call start
	require.Len(t, result[1].Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, "toolu_123", result[1].Choices[0].Delta.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", result[1].Choices[0].Delta.ToolCalls[0].Function.Name)

	// Finish
	finishChunk := result[len(result)-2]
	require.NotNil(t, finishChunk.Choices[0].FinishReason)
	assert.Equal(t, "tool_calls", *finishChunk.Choices[0].FinishReason)
}

func TestAnthropicStream_ThinkingDeltas(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_789","model":"claude-sonnet-4-6","usage":{"input_tokens":10}}}

event: content_block_start
data: {"index":0,"content_block":{"type":"thinking"}}

event: content_block_delta
data: {"index":0,"delta":{"type":"thinking_delta","thinking":"Let me think..."}}

event: content_block_stop
data: {"index":0}

event: content_block_start
data: {"index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"index":1,"delta":{"type":"text_delta","text":"Here's my answer"}}

event: content_block_stop
data: {"index":1}

event: message_delta
data: {"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":30}}

event: message_stop
data: {}

`

	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)
	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var result []*dto.ChatCompletionChunk
	for chunk := range chunks {
		result = append(result, chunk)
	}

	// Find thinking chunk
	var foundThinking bool
	for _, chunk := range result {
		if chunk != nil && len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Thinking != "" {
			assert.Equal(t, "Let me think...", chunk.Choices[0].Delta.Thinking)
			foundThinking = true
		}
	}
	assert.True(t, foundThinking, "should have a thinking delta chunk")
}

func TestAnthropicStream_Ping(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	sseData := `event: ping
data: {}

event: message_start
data: {"type":"message_start","message":{"id":"msg_100","model":"claude-sonnet-4-6","usage":{"input_tokens":5}}}

event: message_delta
data: {"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}

event: message_stop
data: {}

`

	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)
	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var result []*dto.ChatCompletionChunk
	for chunk := range chunks {
		result = append(result, chunk)
	}

	// Ping should be forwarded as sentinel, followed by role + finish + done
	require.GreaterOrEqual(t, len(result), 4)
	assert.Equal(t, "ping", result[0].Object)
	assert.Equal(t, "assistant", result[1].Choices[0].Delta.Role)
}

func TestAnthropicStream_MaxTokens(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_200","model":"claude-sonnet-4-6","usage":{"input_tokens":5}}}

event: message_delta
data: {"delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":100}}

event: message_stop
data: {}

`

	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)
	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var result []*dto.ChatCompletionChunk
	for chunk := range chunks {
		result = append(result, chunk)
	}

	// Find finish chunk
	for _, chunk := range result {
		if chunk != nil && len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != nil {
			assert.Equal(t, "length", *chunk.Choices[0].FinishReason)
			return
		}
	}
	t.Fatal("no finish chunk with length reason found")
}

// Prompt caching telemetry tests (Stage 2)

func TestAnthropicStream_CacheTokensSurfacedInUsage(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_cache","model":"claude-sonnet-4-6","usage":{"input_tokens":12,"cache_creation_input_tokens":500,"cache_read_input_tokens":3000}}}

event: content_block_start
data: {"index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"index":0,"delta":{"type":"text_delta","text":"cached!"}}

event: content_block_stop
data: {"index":0}

event: message_delta
data: {"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}

event: message_stop
data: {}

`

	result := collectChunks(t, adapter, sseData)

	// Locate the final usage-bearing chunk
	var finalUsage *dto.Usage
	for _, c := range result {
		if c != nil && c.Usage != nil {
			finalUsage = c.Usage
		}
	}
	require.NotNil(t, finalUsage, "expected a Usage chunk")
	require.NotNil(t, finalUsage.PromptTokensDetails,
		"PromptTokensDetails must be populated when cache activity occurred")
	assert.Equal(t, 3000, finalUsage.PromptTokensDetails.CachedTokens,
		"cache_read_input_tokens should map to cached_tokens")
	assert.Equal(t, 500, finalUsage.PromptTokensDetails.CacheCreationTokens,
		"cache_creation_input_tokens should map to cache_creation_tokens")
}

func TestAnthropicStream_NoCacheActivity_OmitsDetails(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	// Same shape as the basic text response test — zero cache fields.
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_nocache","model":"claude-sonnet-4-6","usage":{"input_tokens":10}}}

event: content_block_start
data: {"index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"index":0,"delta":{"type":"text_delta","text":"hi"}}

event: content_block_stop
data: {"index":0}

event: message_delta
data: {"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}

event: message_stop
data: {}

`

	result := collectChunks(t, adapter, sseData)

	var finalUsage *dto.Usage
	for _, c := range result {
		if c != nil && c.Usage != nil {
			finalUsage = c.Usage
		}
	}
	require.NotNil(t, finalUsage)
	assert.Nil(t, finalUsage.PromptTokensDetails,
		"PromptTokensDetails must be nil when no cache activity occurred")
}
