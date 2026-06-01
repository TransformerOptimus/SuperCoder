package impl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

func TestOpenAICompatAdapter_Name(t *testing.T) {
	adapter := NewOpenAICompatAdapter("local", "http://localhost:11434/v1", "", []string{"llama-"}, zap.NewNop())
	assert.Equal(t, "local", adapter.Name())
}

func TestOpenAICompatAdapter_MatchesModel(t *testing.T) {
	adapter := NewOpenAICompatAdapter("local", "http://localhost:11434/v1", "", []string{"llama-", "deepseek-"}, zap.NewNop())
	assert.True(t, adapter.MatchesModel("llama-3.1"))
	assert.True(t, adapter.MatchesModel("deepseek-v2"))
	assert.False(t, adapter.MatchesModel("gpt-5.4"))
}

func TestOpenAICompatAdapter_PassthroughRequest(t *testing.T) {
	adapter := NewOpenAICompatAdapter("local", "http://localhost:11434/v1", "", []string{"llama-"}, zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "llama-3.1",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		Stream: true,
	}

	url, headers, body, err := adapter.TranslateRequest(req, "")
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:11434/v1/chat/completions", url)
	assert.Equal(t, "application/json", headers["Content-Type"])
	_, hasAuth := headers["Authorization"]
	assert.False(t, hasAuth, "no auth header when key is empty")

	// Body should be the original request marshaled as-is
	var roundTripped dto.ChatCompletionRequest
	require.NoError(t, json.Unmarshal(body, &roundTripped))
	assert.Equal(t, "llama-3.1", roundTripped.Model)
	assert.True(t, roundTripped.Stream)
}

func TestOpenAICompatAdapter_WithAPIKey(t *testing.T) {
	adapter := NewOpenAICompatAdapter("local", "http://localhost:11434/v1", "", []string{"llama-"}, zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "llama-3.1",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
	}

	_, headers, _, err := adapter.TranslateRequest(req, "my-key")
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-key", headers["Authorization"])
}

func TestOpenAICompatAdapter_StreamPassthrough(t *testing.T) {
	adapter := NewOpenAICompatAdapter("local", "http://localhost:11434/v1", "", []string{"llama-"}, zap.NewNop())

	sseData := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","model":"llama-3.1","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","model":"llama-3.1","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","model":"llama-3.1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`

	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)
	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var result []*dto.ChatCompletionChunk
	for chunk := range chunks {
		result = append(result, chunk)
	}

	// role, "Hi", finish, nil (DONE)
	require.Len(t, result, 4)

	assert.Equal(t, "assistant", result[0].Choices[0].Delta.Role)
	assert.Equal(t, "Hi", result[1].Choices[0].Delta.Content)
	require.NotNil(t, result[2].Choices[0].FinishReason)
	assert.Equal(t, "stop", *result[2].Choices[0].FinishReason)
	assert.Nil(t, result[3])
}

// Verify cached_tokens survives passthrough.

func TestOpenAICompatAdapter_PreservesCachedTokens(t *testing.T) {
	adapter := NewOpenAICompatAdapter("local", "http://localhost:11434/v1", "", []string{"llama-"}, zap.NewNop())

	sseData := `data: {"id":"c1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"c1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2006,"completion_tokens":300,"total_tokens":2306,"prompt_tokens_details":{"cached_tokens":1920}}}

data: [DONE]

`

	chunks := make(chan *dto.ChatCompletionChunk, 64)
	reader := strings.NewReader(sseData)
	err := adapter.TranslateStream(context.Background(), reader, chunks)
	require.NoError(t, err)

	var finalUsage *dto.Usage
	for c := range chunks {
		if c != nil && c.Usage != nil {
			finalUsage = c.Usage
		}
	}
	require.NotNil(t, finalUsage)
	require.NotNil(t, finalUsage.PromptTokensDetails,
		"passthrough must not drop prompt_tokens_details")
	assert.Equal(t, 1920, finalUsage.PromptTokensDetails.CachedTokens)
}
