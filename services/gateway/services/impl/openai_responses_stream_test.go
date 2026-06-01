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

func TestOpenAIResponsesStream_TextResponse(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	sseData := `event: response.created
data: {"response":{"id":"resp_123","model":"gpt-5.4"}}

event: response.output_item.added
data: {"output_index":0,"item":{"type":"message","id":"msg_1"}}

event: response.output_text.delta
data: {"output_index":0,"content_index":0,"delta":"Hello"}

event: response.output_text.delta
data: {"output_index":0,"content_index":0,"delta":" world"}

event: response.completed
data: {"response":{"id":"resp_123","model":"gpt-5.4","status":"completed","output":[{"type":"message"}],"usage":{"input_tokens":10,"output_tokens":5}}}

`

	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)
	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var result []*dto.ChatCompletionChunk
	for chunk := range chunks {
		result = append(result, chunk)
	}

	// role, "Hello", " world", finish, nil
	require.GreaterOrEqual(t, len(result), 4)

	assert.Equal(t, "assistant", result[0].Choices[0].Delta.Role)
	assert.Equal(t, "resp_123", result[0].ID)

	assert.Equal(t, "Hello", result[1].Choices[0].Delta.Content)
	assert.Equal(t, " world", result[2].Choices[0].Delta.Content)

	// Finish chunk
	finishIdx := len(result) - 2
	require.NotNil(t, result[finishIdx].Choices[0].FinishReason)
	assert.Equal(t, "stop", *result[finishIdx].Choices[0].FinishReason)
	require.NotNil(t, result[finishIdx].Usage)
	assert.Equal(t, 10, result[finishIdx].Usage.PromptTokens)
	assert.Equal(t, 5, result[finishIdx].Usage.CompletionTokens)

	// DONE
	assert.Nil(t, result[len(result)-1])
}

func TestOpenAIResponsesStream_FunctionCall(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	sseData := `event: response.created
data: {"response":{"id":"resp_456","model":"gpt-5.4"}}

event: response.output_item.added
data: {"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"get_weather"}}

event: response.function_call_arguments.delta
data: {"output_index":0,"delta":"{\"city\":"}

event: response.function_call_arguments.delta
data: {"output_index":0,"delta":"\"NYC\"}"}

event: response.completed
data: {"response":{"id":"resp_456","model":"gpt-5.4","status":"completed","output":[{"type":"function_call"}],"usage":{"input_tokens":20,"output_tokens":15}}}

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

	// Role
	assert.Equal(t, "assistant", result[0].Choices[0].Delta.Role)

	// Tool call start
	require.Len(t, result[1].Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, "call_abc", result[1].Choices[0].Delta.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", result[1].Choices[0].Delta.ToolCalls[0].Function.Name)

	// Finish with tool_calls reason
	finishIdx := len(result) - 2
	require.NotNil(t, result[finishIdx].Choices[0].FinishReason)
	assert.Equal(t, "tool_calls", *result[finishIdx].Choices[0].FinishReason)
}

func TestOpenAIResponsesStream_ReasoningSummary(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	sseData := `event: response.created
data: {"response":{"id":"resp_789","model":"o3"}}

event: response.reasoning_summary_text.delta
data: {"output_index":0,"delta":"Thinking about..."}

event: response.output_text.delta
data: {"output_index":0,"content_index":0,"delta":"Answer"}

event: response.completed
data: {"response":{"id":"resp_789","model":"o3","status":"completed","output":[{"type":"message"}],"usage":{"input_tokens":5,"output_tokens":10,"reasoning_tokens":50}}}

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
			assert.Equal(t, "Thinking about...", chunk.Choices[0].Delta.Thinking)
			foundThinking = true
		}
	}
	assert.True(t, foundThinking, "should have a thinking/reasoning delta chunk")
}

func TestOpenAIResponsesStream_Incomplete(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	sseData := `event: response.created
data: {"response":{"id":"resp_inc","model":"gpt-5.4"}}

event: response.incomplete
data: {"response":{"id":"resp_inc","model":"gpt-5.4","status":"incomplete"}}

`

	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)
	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var result []*dto.ChatCompletionChunk
	for chunk := range chunks {
		result = append(result, chunk)
	}

	// Should have role, length finish, nil
	require.GreaterOrEqual(t, len(result), 2)

	// Find finish with length
	for _, chunk := range result {
		if chunk != nil && len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != nil {
			assert.Equal(t, "length", *chunk.Choices[0].FinishReason)
			return
		}
	}
	t.Fatal("no finish chunk with length reason found")
}

// Verify input_tokens_details.cached_tokens survives /responses translation.

func TestOpenAIResponsesStream_CachedTokensSurfaced(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	sseData := `event: response.created
data: {"response":{"id":"resp_cache","model":"gpt-5.4"}}

event: response.output_item.added
data: {"output_index":0,"item":{"type":"message","id":"msg_1"}}

event: response.output_text.delta
data: {"output_index":0,"content_index":0,"delta":"hi"}

event: response.completed
data: {"response":{"id":"resp_cache","model":"gpt-5.4","status":"completed","output":[{"type":"message"}],"usage":{"input_tokens":2006,"input_tokens_details":{"cached_tokens":1920},"output_tokens":300}}}

`

	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)
	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var finalUsage *dto.Usage
	for chunk := range chunks {
		if chunk != nil && chunk.Usage != nil {
			finalUsage = chunk.Usage
		}
	}
	require.NotNil(t, finalUsage, "expected a Usage chunk in responses stream")
	require.NotNil(t, finalUsage.PromptTokensDetails,
		"PromptTokensDetails must be set when OpenAI reports cached_tokens")
	assert.Equal(t, 1920, finalUsage.PromptTokensDetails.CachedTokens)

	assert.Equal(t, 0, finalUsage.PromptTokensDetails.CacheCreationTokens)
}
