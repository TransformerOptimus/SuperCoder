package impl

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

// OpenAIResponsesAdapter translates Chat Completions format to/from OpenAI Responses API.
type OpenAIResponsesAdapter struct {
	baseURL string
	apiKey  string
	logger  *zap.Logger
}

func NewOpenAIResponsesAdapter(baseURL, apiKey string, logger *zap.Logger) *OpenAIResponsesAdapter {
	return &OpenAIResponsesAdapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		logger:  logger.Named("gateway.openai-responses"),
	}
}

func (o *OpenAIResponsesAdapter) ConfiguredAPIKey() string {
	return o.apiKey
}

func (o *OpenAIResponsesAdapter) Name() string {
	return "openai"
}

func (o *OpenAIResponsesAdapter) MatchesModel(model string) bool {
	prefixes := []string{"gpt-", "o1-", "o3-", "o4-", "chatgpt-"}
	for _, p := range prefixes {
		if strings.HasPrefix(model, p) {
			return true
		}
	}
	return false
}

func (o *OpenAIResponsesAdapter) TranslateRequest(req *dto.ChatCompletionRequest, apiKey string) (string, map[string]string, []byte, error) {
	respReq := dto.ResponsesRequest{
		Model:  req.Model,
		Stream: true,
		Store:  false,
	}

	// Extract system messages → instructions
	var instructions []string
	var inputItems []json.RawMessage

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			text, _, _ := dto.ParseMessageContent(msg.Content)
			if text != "" {
				instructions = append(instructions, text)
			}
			continue
		}

		items, err := translateMessageToResponsesInput(msg)
		if err != nil {
			return "", nil, nil, fmt.Errorf("translate message: %w", err)
		}
		inputItems = append(inputItems, items...)
	}

	if len(instructions) > 0 {
		respReq.Instructions = strings.Join(instructions, "\n\n")
	}

	inputBytes, err := json.Marshal(inputItems)
	if err != nil {
		return "", nil, nil, fmt.Errorf("marshal input: %w", err)
	}
	respReq.Input = inputBytes

	// Tools
	if len(req.Tools) > 0 {
		for _, t := range req.Tools {
			respReq.Tools = append(respReq.Tools, dto.ResponsesTool{
				Type:        "function",
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
	}

	// Max tokens
	if req.MaxCompletionTokens != nil {
		respReq.MaxOutputTokens = req.MaxCompletionTokens
	}

	// Temperature
	if req.Temperature != nil {
		respReq.Temperature = req.Temperature
	}

	// Tool choice
	if len(req.ToolChoice) > 0 {
		respReq.ToolChoice = translateToolChoiceForResponses(req.ToolChoice)
	}

	// Thinking → reasoning
	if req.Thinking != nil && req.Thinking.BudgetTokens > 0 {
		effort := "high"
		if req.Thinking.BudgetTokens < 5000 {
			effort = "low"
		} else if req.Thinking.BudgetTokens < 20000 {
			effort = "medium"
		}
		respReq.Reasoning = &dto.ResponsesReasoning{
			Effort:  effort,
			Summary: "auto",
		}
	}

	body, err := json.Marshal(respReq)
	if err != nil {
		return "", nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := o.baseURL + "/responses"
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}

	return url, headers, body, nil
}

func translateMessageToResponsesInput(msg dto.Message) ([]json.RawMessage, error) {
	var items []json.RawMessage

	switch msg.Role {
	case "user":
		item := map[string]any{
			"role": "user",
		}
		text, blocks, _ := dto.ParseMessageContent(msg.Content)
		if len(blocks) > 0 {
			var contentItems []map[string]any
			for _, b := range blocks {
				switch b.Type {
				case "text":
					contentItems = append(contentItems, map[string]any{
						"type": "input_text",
						"text": b.Text,
					})
				case "image_url":
					if b.ImageURL != nil {
						contentItems = append(contentItems, map[string]any{
							"type":      "input_image",
							"image_url": b.ImageURL.URL,
						})
					}
				}
			}
			item["content"] = contentItems
		} else {
			item["content"] = text
		}
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("marshal user input item: %w", err)
		}
		items = append(items, raw)

	case "assistant":
		text, _, _ := dto.ParseMessageContent(msg.Content)
		if text != "" {
			raw, err := json.Marshal(map[string]any{
				"role":    "assistant",
				"content": text,
			})
			if err != nil {
				return nil, fmt.Errorf("marshal assistant input item: %w", err)
			}
			items = append(items, raw)
		}
		// Convert tool_calls to function_call items
		for _, tc := range msg.ToolCalls {
			args := tc.Function.Arguments
			if args == "" || !json.Valid([]byte(args)) {
				args = "{}"
			}
			raw, err := json.Marshal(dto.FunctionCallItem{
				Type:      "function_call",
				CallID:    sanitizeToolID(tc.ID),
				Name:      tc.Function.Name,
				Arguments: args,
			})
			if err != nil {
				return nil, fmt.Errorf("marshal function call item: %w", err)
			}
			items = append(items, raw)
		}

	case "tool":
		text, _, _ := dto.ParseMessageContent(msg.Content)
		raw, err := json.Marshal(dto.FunctionCallOutputItem{
			Type:   "function_call_output",
			CallID: sanitizeToolID(msg.ToolCallID),
			Output: text,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal function call output item: %w", err)
		}
		items = append(items, raw)

	case "developer":
		text, _, _ := dto.ParseMessageContent(msg.Content)
		raw, err := json.Marshal(map[string]any{
			"role":    "developer",
			"content": text,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal developer input item: %w", err)
		}
		items = append(items, raw)
	}

	return items, nil
}

func translateToolChoiceForResponses(raw json.RawMessage) json.RawMessage {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		// "auto", "required", "none" pass through directly as strings
		result, err := json.Marshal(s)
		if err != nil {
			return raw
		}
		return result
	}

	// Object form: {type:"function", function:{name:"X"}} → {type:"function", name:"X"}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Function.Name != "" {
		result, err := json.Marshal(map[string]string{
			"type": "function",
			"name": obj.Function.Name,
		})
		if err != nil {
			return raw
		}
		return result
	}

	return raw
}
