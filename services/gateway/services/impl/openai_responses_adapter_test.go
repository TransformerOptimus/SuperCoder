package impl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

func TestOpenAIResponsesAdapter_Name(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())
	assert.Equal(t, "openai", adapter.Name())
}

func TestOpenAIResponsesAdapter_MatchesModel(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())
	assert.True(t, adapter.MatchesModel("gpt-5.4"))
	assert.True(t, adapter.MatchesModel("o3-mini"))
	assert.True(t, adapter.MatchesModel("o4-mini"))
	assert.False(t, adapter.MatchesModel("claude-sonnet-4-6"))
}

func TestOpenAIResponsesAdapter_BasicRequest(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		Stream: true,
	}

	url, headers, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	assert.Equal(t, "https://api.openai.com/v1/responses", url)
	assert.Equal(t, "Bearer sk-test", headers["Authorization"])

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))
	assert.Equal(t, "gpt-5.4", respReq.Model)
	assert.True(t, respReq.Stream)
	assert.False(t, respReq.Store)
}

func TestOpenAIResponsesAdapter_SystemInstructions(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "system", Content: json.RawMessage(`"You are helpful."`)},
			{Role: "system", Content: json.RawMessage(`"Be concise."`)},
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))
	assert.Equal(t, "You are helpful.\n\nBe concise.", respReq.Instructions)
}

func TestOpenAIResponsesAdapter_ToolCalls(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"What's the weather?"`)},
		},
		Tools: []dto.Tool{
			{
				Type: "function",
				Function: dto.FunctionDef{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))
	require.Len(t, respReq.Tools, 1)
	assert.Equal(t, "function", respReq.Tools[0].Type)
	assert.Equal(t, "get_weather", respReq.Tools[0].Name)
}

func TestOpenAIResponsesAdapter_ToolResults(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"What's the weather?"`)},
			{
				Role: "assistant",
				ToolCalls: []dto.ToolCall{
					{
						ID:   "call_abc",
						Type: "function",
						Function: dto.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"city":"NYC"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    json.RawMessage(`"72°F"`),
				ToolCallID: "call_abc",
			},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))

	// Input should contain: user message, function_call item, function_call_output item
	var items []json.RawMessage
	require.NoError(t, json.Unmarshal(respReq.Input, &items))
	require.Len(t, items, 3)

	// Check function_call item
	var fcItem dto.FunctionCallItem
	require.NoError(t, json.Unmarshal(items[1], &fcItem))
	assert.Equal(t, "function_call", fcItem.Type)
	assert.Equal(t, "call_abc", fcItem.CallID)

	// Check function_call_output item
	var fcoItem dto.FunctionCallOutputItem
	require.NoError(t, json.Unmarshal(items[2], &fcoItem))
	assert.Equal(t, "function_call_output", fcoItem.Type)
	assert.Equal(t, "call_abc", fcoItem.CallID)
	assert.Equal(t, "72°F", fcoItem.Output)
}

func TestOpenAIResponsesAdapter_MaxTokens(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	maxTokens := 1024
	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		MaxCompletionTokens: &maxTokens,
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))
	require.NotNil(t, respReq.MaxOutputTokens)
	assert.Equal(t, 1024, *respReq.MaxOutputTokens)
}

func TestOpenAIResponsesAdapter_Thinking(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "o3",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Solve this"`)},
		},
		Thinking: &dto.ThinkingConfig{
			BudgetTokens: 30000,
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))
	require.NotNil(t, respReq.Reasoning)
	assert.Equal(t, "high", respReq.Reasoning.Effort)
	assert.Equal(t, "auto", respReq.Reasoning.Summary)
}

func TestOpenAIResponsesAdapter_ThinkingMedium(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "o3",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		Thinking: &dto.ThinkingConfig{
			BudgetTokens: 10000,
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))
	require.NotNil(t, respReq.Reasoning)
	assert.Equal(t, "medium", respReq.Reasoning.Effort)
}

func TestOpenAIResponsesAdapter_ToolChoiceString(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		ToolChoice: json.RawMessage(`"auto"`),
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))

	var tc string
	require.NoError(t, json.Unmarshal(respReq.ToolChoice, &tc))
	assert.Equal(t, "auto", tc)
}

func TestOpenAIResponsesAdapter_ToolChoiceObject(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		ToolChoice: json.RawMessage(`{"type":"function","function":{"name":"get_weather"}}`),
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))

	var tc map[string]string
	require.NoError(t, json.Unmarshal(respReq.ToolChoice, &tc))
	assert.Equal(t, "function", tc["type"])
	assert.Equal(t, "get_weather", tc["name"])
}

func TestOpenAIResponsesAdapter_ThinkingLow(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "o3",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		Thinking: &dto.ThinkingConfig{BudgetTokens: 2000},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))
	require.NotNil(t, respReq.Reasoning)
	assert.Equal(t, "low", respReq.Reasoning.Effort)
}

func TestOpenAIResponsesAdapter_DeveloperRole(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "developer", Content: json.RawMessage(`"You must always respond in JSON."`)},
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))

	var items []json.RawMessage
	require.NoError(t, json.Unmarshal(respReq.Input, &items))
	require.Len(t, items, 2)

	var devMsg map[string]any
	require.NoError(t, json.Unmarshal(items[0], &devMsg))
	assert.Equal(t, "developer", devMsg["role"])
}

func TestOpenAIResponsesAdapter_ImageContent(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"text","text":"What's this?"},
					{"type":"image_url","image_url":{"url":"data:image/png;base64,abc123"}}
				]`),
			},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))

	var items []json.RawMessage
	require.NoError(t, json.Unmarshal(respReq.Input, &items))
	require.Len(t, items, 1)

	var msg map[string]any
	require.NoError(t, json.Unmarshal(items[0], &msg))
	content := msg["content"].([]any)
	require.Len(t, content, 2)

	imgItem := content[1].(map[string]any)
	assert.Equal(t, "input_image", imgItem["type"])
	assert.Equal(t, "data:image/png;base64,abc123", imgItem["image_url"])
}

func TestOpenAIResponsesAdapter_TruncatedToolCallArguments(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Do something"`)},
			{
				Role: "assistant",
				ToolCalls: []dto.ToolCall{
					{ID: "call_1", Type: "function", Function: dto.FunctionCall{Name: "run_tool", Arguments: `{"city":"NY`}},
				},
			},
			{Role: "tool", Content: json.RawMessage(`"result"`), ToolCallID: "call_1"},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))

	var items []json.RawMessage
	require.NoError(t, json.Unmarshal(respReq.Input, &items))
	require.Len(t, items, 3)

	var fcItem dto.FunctionCallItem
	require.NoError(t, json.Unmarshal(items[1], &fcItem))
	assert.Equal(t, "function_call", fcItem.Type)
	assert.Equal(t, "{}", fcItem.Arguments)
}

func TestOpenAIResponsesAdapter_ToolIDSanitization(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Do something"`)},
			{
				Role: "assistant",
				ToolCalls: []dto.ToolCall{
					{ID: "call!@#abc", Type: "function", Function: dto.FunctionCall{Name: "run_tool", Arguments: `{"a":"b"}`}},
				},
			},
			{Role: "tool", Content: json.RawMessage(`"result"`), ToolCallID: "call!@#abc"},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))

	var items []json.RawMessage
	require.NoError(t, json.Unmarshal(respReq.Input, &items))
	require.Len(t, items, 3)

	var fcItem dto.FunctionCallItem
	require.NoError(t, json.Unmarshal(items[1], &fcItem))
	assert.Equal(t, "call___abc", fcItem.CallID)

	var fcoItem dto.FunctionCallOutputItem
	require.NoError(t, json.Unmarshal(items[2], &fcoItem))
	assert.Equal(t, "call___abc", fcoItem.CallID)
}

func TestOpenAIResponsesAdapter_NoThinkingWhenZeroBudget(t *testing.T) {
	adapter := NewOpenAIResponsesAdapter("https://api.openai.com/v1", "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "gpt-5.4",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		Thinking: &dto.ThinkingConfig{BudgetTokens: 0},
	}

	_, _, body, err := adapter.TranslateRequest(req, "sk-test")
	require.NoError(t, err)

	var respReq dto.ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &respReq))
	assert.Nil(t, respReq.Reasoning, "zero budget should not set reasoning")
}
