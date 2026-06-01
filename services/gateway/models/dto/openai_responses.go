package dto

import "encoding/json"

// ResponsesRequest is the outbound request to OpenAI's Responses API.
type ResponsesRequest struct {
	Model           string              `json:"model"`
	Input           json.RawMessage     `json:"input"`
	Instructions    string              `json:"instructions,omitempty"`
	Tools           []ResponsesTool     `json:"tools,omitempty"`
	Stream          bool                `json:"stream,omitempty"`
	MaxOutputTokens *int                `json:"max_output_tokens,omitempty"`
	Temperature     *float64            `json:"temperature,omitempty"`
	ToolChoice      json.RawMessage     `json:"tool_choice,omitempty"`
	Store           bool                `json:"store"`
	Reasoning       *ResponsesReasoning `json:"reasoning,omitempty"`
}

type ResponsesReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type ResponsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// ResponsesInputMessage is a message item in the input array.
type ResponsesInputMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// FunctionCallItem represents a function_call input item.
type FunctionCallItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// FunctionCallOutputItem represents a function_call_output input item.
type FunctionCallOutputItem struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// Responses API SSE event types

type ResponseCreatedEvent struct {
	Response ResponseCreatedData `json:"response"`
}

type ResponseCreatedData struct {
	ID    string `json:"id"`
	Model string `json:"model"`
}

type OutputItemAddedEvent struct {
	OutputIndex int                 `json:"output_index"`
	Item        OutputItemAddedItem `json:"item"`
}

type OutputItemAddedItem struct {
	Type   string `json:"type"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	CallID string `json:"call_id,omitempty"`
}

type OutputTextDeltaEvent struct {
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

type FunctionCallArgsDeltaEvent struct {
	OutputIndex int    `json:"output_index"`
	Delta       string `json:"delta"`
}

type ReasoningSummaryDeltaEvent struct {
	OutputIndex int    `json:"output_index"`
	Delta       string `json:"delta"`
}

type ResponseCompletedEvent struct {
	Response ResponseCompletedResponse `json:"response"`
}

type ResponseCompletedResponse struct {
	ID     string          `json:"id"`
	Model  string          `json:"model"`
	Status string          `json:"status"`
	Output json.RawMessage `json:"output"`
	Usage  *ResponsesUsage `json:"usage,omitempty"`
}

type ResponsesUsage struct {
	InputTokens        int                          `json:"input_tokens"`
	InputTokensDetails *ResponsesInputTokensDetails `json:"input_tokens_details,omitempty"`
	OutputTokens       int                          `json:"output_tokens"`
	ReasoningTokens    int                          `json:"reasoning_tokens,omitempty"`
}

type ResponsesInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

type ResponseErrorEvent struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// ResponseOutputItem is used to inspect output items in response.completed
type ResponseOutputItem struct {
	Type string `json:"type"`
}
