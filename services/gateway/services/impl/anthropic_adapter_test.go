package impl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

func TestAnthropicAdapter_Name(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())
	assert.Equal(t, "anthropic", adapter.Name())
}

func TestAnthropicAdapter_MatchesModel(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())
	assert.True(t, adapter.MatchesModel("claude-sonnet-4-6"))
	assert.True(t, adapter.MatchesModel("claude-opus-4-6"))
	assert.False(t, adapter.MatchesModel("gpt-5.4"))
}

func TestAnthropicAdapter_BasicRequest(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		Stream: true,
	}

	url, headers, body, err := adapter.TranslateRequest(req, "test-key")
	require.NoError(t, err)

	assert.Equal(t, "https://api.anthropic.com/v1/messages", url)
	assert.Equal(t, "test-key", headers["x-api-key"])
	assert.Equal(t, "2023-06-01", headers["anthropic-version"])

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	assert.Equal(t, "claude-sonnet-4-6", anthropicReq.Model)
	assert.True(t, anthropicReq.Stream)
	assert.Equal(t, 8192, anthropicReq.MaxTokens)
	assert.Len(t, anthropicReq.Messages, 1)
}

func TestAnthropicAdapter_SystemPrompt(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "system", Content: json.RawMessage(`"You are a helpful assistant."`)},
			{Role: "system", Content: json.RawMessage(`"Be concise."`)},
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	assert.Equal(t, "You are a helpful assistant.\n\nBe concise.", anthropicReq.System)
	assert.Len(t, anthropicReq.Messages, 1)
	assert.Equal(t, "user", anthropicReq.Messages[0].Role)
}

func TestAnthropicAdapter_MaxTokens(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	maxTokens := 4096
	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		MaxCompletionTokens: &maxTokens,
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	assert.Equal(t, 4096, anthropicReq.MaxTokens)
}

func TestAnthropicAdapter_Temperature(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	temp := 0.7
	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		Temperature: &temp,
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	require.NotNil(t, anthropicReq.Temperature)
	assert.InDelta(t, 0.7, *anthropicReq.Temperature, 0.001)
}

func TestAnthropicAdapter_ToolCalls(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"What is the weather?"`)},
		},
		Tools: []dto.Tool{
			{
				Type: "function",
				Function: dto.FunctionDef{
					Name:        "get_weather",
					Description: "Get the weather",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
				},
			},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	require.Len(t, anthropicReq.Tools, 1)
	assert.Equal(t, "get_weather", anthropicReq.Tools[0].Name)
	assert.Equal(t, "Get the weather", anthropicReq.Tools[0].Description)
}

func TestAnthropicAdapter_ToolChoice(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	tests := []struct {
		name       string
		toolChoice json.RawMessage
		expected   string
	}{
		{"auto", json.RawMessage(`"auto"`), "auto"},
		{"required", json.RawMessage(`"required"`), "any"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &dto.ChatCompletionRequest{
				Model: "claude-sonnet-4-6",
				Messages: []dto.Message{
					{Role: "user", Content: json.RawMessage(`"Hello"`)},
				},
				ToolChoice: tt.toolChoice,
			}

			_, _, body, err := adapter.TranslateRequest(req, "key")
			require.NoError(t, err)

			var anthropicReq dto.AnthropicRequest
			require.NoError(t, json.Unmarshal(body, &anthropicReq))
			require.NotNil(t, anthropicReq.ToolChoice)
			assert.Equal(t, tt.expected, anthropicReq.ToolChoice.Type)
		})
	}
}

func TestAnthropicAdapter_DisableParallelToolCalls(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	parallel := false
	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		ToolChoice:        json.RawMessage(`"auto"`),
		ParallelToolCalls: &parallel,
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	require.NotNil(t, anthropicReq.ToolChoice)
	assert.True(t, anthropicReq.ToolChoice.DisableParallelToolUse)
}

func TestAnthropicAdapter_AssistantWithToolCalls(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"What is the weather?"`)},
			{
				Role:    "assistant",
				Content: json.RawMessage(`"Let me check the weather."`),
				ToolCalls: []dto.ToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: dto.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location":"NYC"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    json.RawMessage(`"72°F and sunny"`),
				ToolCallID: "call_123",
			},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))

	// Should have: user, assistant (with text + tool_use), user (with tool_result)
	require.Len(t, anthropicReq.Messages, 3)
	assert.Equal(t, "user", anthropicReq.Messages[0].Role)
	assert.Equal(t, "assistant", anthropicReq.Messages[1].Role)
	assert.Equal(t, "user", anthropicReq.Messages[2].Role)

	// Check assistant message has tool_use block
	var assistantBlocks []dto.AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[1].Content, &assistantBlocks))
	require.Len(t, assistantBlocks, 2)
	assert.Equal(t, "text", assistantBlocks[0].Type)
	assert.Equal(t, "tool_use", assistantBlocks[1].Type)
	assert.Equal(t, "call_123", assistantBlocks[1].ID)

	// Check tool_result
	var toolBlocks []dto.AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[2].Content, &toolBlocks))
	require.Len(t, toolBlocks, 1)
	assert.Equal(t, "tool_result", toolBlocks[0].Type)
	assert.Equal(t, "call_123", toolBlocks[0].ToolUseID)
}

func TestAnthropicAdapter_Thinking(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Solve this problem"`)},
		},
		Thinking: &dto.ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: 10000,
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	require.NotNil(t, anthropicReq.Thinking)
	assert.Equal(t, "enabled", anthropicReq.Thinking.Type)
	assert.Equal(t, 10000, anthropicReq.Thinking.BudgetTokens)
	// Temperature must be 1.0 with thinking
	require.NotNil(t, anthropicReq.Temperature)
	assert.InDelta(t, 1.0, *anthropicReq.Temperature, 0.001)
}

func TestAnthropicAdapter_StopSequences(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		Stop: json.RawMessage(`["STOP", "END"]`),
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	assert.Equal(t, []string{"STOP", "END"}, anthropicReq.StopSequences)
}

func TestAnthropicAdapter_StopSequencesString(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		Stop: json.RawMessage(`"STOP"`),
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	assert.Equal(t, []string{"STOP"}, anthropicReq.StopSequences)
}

func TestAnthropicAdapter_MessageAdjacency(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	// Two consecutive user messages should be merged
	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
			{Role: "user", Content: json.RawMessage(`"How are you?"`)},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	assert.Len(t, anthropicReq.Messages, 1, "consecutive user messages should be merged")
}

func TestAnthropicAdapter_ImageContent(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type": "text", "text": "What's in this image?"},
					{"type": "image_url", "image_url": {"url": "data:image/png;base64,iVBORw0KGgo="}}
				]`),
			},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	require.Len(t, anthropicReq.Messages, 1)

	var blocks []dto.AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[0].Content, &blocks))
	require.Len(t, blocks, 2)
	assert.Equal(t, "text", blocks[0].Type)
	assert.Equal(t, "image", blocks[1].Type)
	require.NotNil(t, blocks[1].Source)
	assert.Equal(t, "base64", blocks[1].Source.Type)
	assert.Equal(t, "image/png", blocks[1].Source.MediaType)
}

func TestAnthropicAdapter_DefaultMaxTokens(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 0, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	assert.Equal(t, 8192, anthropicReq.MaxTokens, "should default to 8192")
}

func TestExtractSystemMessages(t *testing.T) {
	messages := []dto.Message{
		{Role: "system", Content: json.RawMessage(`"Part 1"`)},
		{Role: "user", Content: json.RawMessage(`"Hello"`)},
		{Role: "system", Content: json.RawMessage(`"Part 2"`)},
	}

	result := extractSystemMessages(messages)
	assert.Equal(t, "Part 1\n\nPart 2", result)
}

func TestParseDataURI(t *testing.T) {
	mediaType, data, err := parseDataURI("data:image/png;base64,iVBORw0KGgo=")
	require.NoError(t, err)
	assert.Equal(t, "image/png", mediaType)
	assert.Equal(t, "iVBORw0KGgo=", data)
}

func TestParseDataURI_Invalid(t *testing.T) {
	_, _, err := parseDataURI("https://example.com/image.png")
	assert.Error(t, err)
}

func TestMapAnthropicStopReason(t *testing.T) {
	assert.Equal(t, "stop", mapAnthropicStopReason("end_turn"))
	assert.Equal(t, "tool_calls", mapAnthropicStopReason("tool_use"))
	assert.Equal(t, "length", mapAnthropicStopReason("max_tokens"))
	assert.Equal(t, "stop", mapAnthropicStopReason("stop_sequence"))
	assert.Equal(t, "stop", mapAnthropicStopReason("unknown"))
}

func TestAnthropicAdapter_MultipleToolCalls(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Get weather and time"`)},
			{
				Role: "assistant",
				ToolCalls: []dto.ToolCall{
					{ID: "call_1", Type: "function", Function: dto.FunctionCall{Name: "get_weather", Arguments: `{"city":"NYC"}`}},
					{ID: "call_2", Type: "function", Function: dto.FunctionCall{Name: "get_time", Arguments: `{"tz":"EST"}`}},
				},
			},
			{Role: "tool", Content: json.RawMessage(`"72°F"`), ToolCallID: "call_1"},
			{Role: "tool", Content: json.RawMessage(`"3:00 PM"`), ToolCallID: "call_2"},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))

	// user, assistant (2 tool_use blocks), user (2 tool_result blocks)
	require.Len(t, anthropicReq.Messages, 3)

	var assistantBlocks []dto.AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[1].Content, &assistantBlocks))
	assert.Len(t, assistantBlocks, 2)
	assert.Equal(t, "tool_use", assistantBlocks[0].Type)
	assert.Equal(t, "tool_use", assistantBlocks[1].Type)

	var toolBlocks []dto.AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[2].Content, &toolBlocks))
	assert.Len(t, toolBlocks, 2)
	assert.Equal(t, "call_1", toolBlocks[0].ToolUseID)
	assert.Equal(t, "call_2", toolBlocks[1].ToolUseID)
}

func TestAnthropicAdapter_NilContent(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
			{Role: "assistant", Content: nil, ToolCalls: []dto.ToolCall{
				{ID: "call_1", Type: "function", Function: dto.FunctionCall{Name: "fn", Arguments: "{}"}},
			}},
			{Role: "tool", Content: json.RawMessage(`"result"`), ToolCallID: "call_1"},
			{Role: "user", Content: json.RawMessage(`"thanks"`)},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	assert.GreaterOrEqual(t, len(anthropicReq.Messages), 3)
}

func TestAnthropicAdapter_ToolChoiceNone(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
		},
		ToolChoice: json.RawMessage(`"none"`),
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	assert.Nil(t, anthropicReq.ToolChoice, "none should result in nil tool_choice")
}

func TestAnthropicAdapter_ToolChoiceSpecificFunction(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
		},
		ToolChoice: json.RawMessage(`{"type":"function","function":{"name":"get_weather"}}`),
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	require.NotNil(t, anthropicReq.ToolChoice)
	assert.Equal(t, "tool", anthropicReq.ToolChoice.Type)
	assert.Equal(t, "get_weather", anthropicReq.ToolChoice.Name)
}

func TestAnthropicAdapter_EmptyToolsArray(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
		},
		Tools: []dto.Tool{},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	assert.Nil(t, anthropicReq.Tools)
}

func TestAnthropicAdapter_AssistantThinkingInHistory(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Solve 2+2"`)},
			{
				Role:     "assistant",
				Content:  json.RawMessage(`"4"`),
				Thinking: json.RawMessage(`"Let me calculate: 2+2=4"`),
			},
			{Role: "user", Content: json.RawMessage(`"And 3+3?"`)},
		},
		Thinking: &dto.ThinkingConfig{Type: "enabled", BudgetTokens: 10000},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))

	// Assistant message should have thinking + text blocks
	var blocks []dto.AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[1].Content, &blocks))
	require.Len(t, blocks, 2)
	assert.Equal(t, "thinking", blocks[0].Type)
	assert.Equal(t, "Let me calculate: 2+2=4", blocks[0].Thinking)
	assert.Equal(t, "text", blocks[1].Type)
}

func TestAnthropicAdapter_TruncatedToolCallArguments(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Do something"`)},
			{
				Role: "assistant",
				ToolCalls: []dto.ToolCall{
					{ID: "call_1", Type: "function", Function: dto.FunctionCall{Name: "run_tool", Arguments: `{"city":"NY`}},
				},
			},
			{Role: "tool", Content: json.RawMessage(`"result"`), ToolCallID: "call_1"},
			{Role: "user", Content: json.RawMessage(`"ok"`)},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))

	var assistantBlocks []dto.AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[1].Content, &assistantBlocks))
	require.Len(t, assistantBlocks, 1)
	assert.Equal(t, "tool_use", assistantBlocks[0].Type)
	assert.JSONEq(t, `{}`, string(assistantBlocks[0].Input))
}

func TestSanitizeToolID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"call_123", "call_123"},
		{"call-abc_def", "call-abc_def"},
		{"call!@#$%", "call_____"},
		{"", ""},
		{"toolu_01ABC", "toolu_01ABC"},
		{"call::123", "call__123"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeToolID(tt.input))
		})
	}
}

func TestAnthropicAdapter_ToolIDSanitization(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"Do something"`)},
			{
				Role: "assistant",
				ToolCalls: []dto.ToolCall{
					{ID: "call!@#abc", Type: "function", Function: dto.FunctionCall{Name: "run_tool", Arguments: `{"a":"b"}`}},
				},
			},
			{Role: "tool", Content: json.RawMessage(`"result"`), ToolCallID: "call!@#abc"},
			{Role: "user", Content: json.RawMessage(`"ok"`)},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))

	var assistantBlocks []dto.AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[1].Content, &assistantBlocks))
	require.Len(t, assistantBlocks, 1)
	assert.Equal(t, "tool_use", assistantBlocks[0].Type)
	assert.Equal(t, "call___abc", assistantBlocks[0].ID)

	var toolResultBlocks []dto.AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anthropicReq.Messages[2].Content, &toolResultBlocks))
	require.GreaterOrEqual(t, len(toolResultBlocks), 1)
	assert.Equal(t, "tool_result", toolResultBlocks[0].Type)
	assert.Equal(t, "call___abc", toolResultBlocks[0].ToolUseID)
}

// Prompt caching tests (Stage 2)

func TestAnthropicAdapter_CacheControlOnLastTool(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
		Tools: []dto.Tool{
			{
				Type:     "function",
				Function: dto.FunctionDef{Name: "t1", Description: "first", Parameters: json.RawMessage(`{}`)},
			},
			{
				Type:         "function",
				Function:     dto.FunctionDef{Name: "t2", Description: "last", Parameters: json.RawMessage(`{}`)},
				CacheControl: &dto.CacheControl{Type: "ephemeral"},
			},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	require.Len(t, anthropicReq.Tools, 2)
	assert.Nil(t, anthropicReq.Tools[0].CacheControl, "first tool must not carry cache_control")
	require.NotNil(t, anthropicReq.Tools[1].CacheControl, "last tool must carry cache_control")
	assert.Equal(t, "ephemeral", anthropicReq.Tools[1].CacheControl.Type)
}

func TestAnthropicAdapter_CacheControlOnSystemBlock(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	// System content is an array of ContentBlocks; only the first carries cache_control.
	systemContent, _ := json.Marshal([]dto.ContentBlock{
		{Type: "text", Text: "static rules", CacheControl: &dto.CacheControl{Type: "ephemeral"}},
		{Type: "text", Text: "dynamic env"},
	})

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "system", Content: systemContent},
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	// Parse as a generic map to inspect the polymorphic `system` field.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))
	require.Contains(t, raw, "system")

	var blocks []dto.AnthropicSystemBlock
	require.NoError(t, json.Unmarshal(raw["system"], &blocks),
		"system must serialize as block array when any block carries cache_control")
	require.Len(t, blocks, 2)
	assert.Equal(t, "static rules", blocks[0].Text)
	require.NotNil(t, blocks[0].CacheControl)
	assert.Equal(t, "ephemeral", blocks[0].CacheControl.Type)
	assert.Equal(t, "dynamic env", blocks[1].Text)
	assert.Nil(t, blocks[1].CacheControl)
}

func TestAnthropicAdapter_TopLevelCacheControl(t *testing.T) {
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model:        "claude-sonnet-4-6",
		Messages:     []dto.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		CacheControl: &dto.CacheControl{Type: "ephemeral"},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var anthropicReq dto.AnthropicRequest
	require.NoError(t, json.Unmarshal(body, &anthropicReq))
	require.NotNil(t, anthropicReq.CacheControl)
	assert.Equal(t, "ephemeral", anthropicReq.CacheControl.Type)
}

func TestAnthropicAdapter_LegacyStringSystem_NoCacheControl(t *testing.T) {
	// Regression: plain-string system with no cache markers must still serialize
	// as a STRING (not an array) — preserves wire-compat with pre-caching callers.
	adapter := NewAnthropicAdapter("https://api.anthropic.com", "2023-06-01", 8192, "", zap.NewNop())

	req := &dto.ChatCompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.Message{
			{Role: "system", Content: json.RawMessage(`"You are helpful."`)},
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}

	_, _, body, err := adapter.TranslateRequest(req, "key")
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))

	// Must be a JSON string, not an array
	var asString string
	require.NoError(t, json.Unmarshal(raw["system"], &asString),
		"system must remain a plain JSON string when no cache_control is used")
	assert.Equal(t, "You are helpful.", asString)
}
