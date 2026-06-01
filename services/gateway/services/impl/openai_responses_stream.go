package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/gateway/services"
)

type responsesStreamState struct {
	responseID           string
	model                string
	toolCallIndex        int
	itemIndexToToolIndex map[int]int
	itemIndexToCallID    map[int]string
	hasEmittedRole       bool
}

// TranslateStream reads OpenAI Responses SSE events and sends Chat Completions chunks.
func (o *OpenAIResponsesAdapter) TranslateStream(ctx context.Context, providerBody io.Reader, chunks chan<- *dto.ChatCompletionChunk) error {
	defer close(chunks)

	state := &responsesStreamState{
		itemIndexToToolIndex: make(map[int]int),
		itemIndexToCallID:    make(map[int]string),
	}

	return services.ParseSSEStream(ctx, providerBody, func(evt services.SSEEvent) error {
		return o.handleResponsesEvent(state, evt, chunks)
	})
}

func (o *OpenAIResponsesAdapter) handleResponsesEvent(state *responsesStreamState, evt services.SSEEvent, chunks chan<- *dto.ChatCompletionChunk) error {
	switch evt.Event {
	case "response.created":
		return o.handleResponseCreated(state, evt.Data, chunks)
	case "response.output_item.added":
		return o.handleOutputItemAdded(state, evt.Data, chunks)
	case "response.output_text.delta":
		return o.handleOutputTextDelta(state, evt.Data, chunks)
	case "response.function_call_arguments.delta":
		return o.handleFunctionCallArgsDelta(state, evt.Data, chunks)
	case "response.reasoning_summary_text.delta":
		return o.handleReasoningSummaryDelta(state, evt.Data, chunks)
	case "response.completed":
		return o.handleResponseCompleted(state, evt.Data, chunks)
	case "response.failed":
		return o.handleResponseFailed(state, evt.Data, chunks)
	case "response.incomplete":
		return o.handleResponseIncomplete(state, chunks)
	default:
		// Drop: content_part.added/done, output_item.done, etc.
		return nil
	}
}

func (o *OpenAIResponsesAdapter) handleResponseCreated(state *responsesStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var evt dto.ResponseCreatedEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return fmt.Errorf("unmarshal response.created: %w", err)
	}

	state.responseID = evt.Response.ID
	state.model = evt.Response.Model

	if !state.hasEmittedRole {
		state.hasEmittedRole = true
		chunks <- &dto.ChatCompletionChunk{
			ID:     state.responseID,
			Object: "chat.completion.chunk",
			Model:  state.model,
			Choices: []dto.ChunkChoice{{
				Index: 0,
				Delta: dto.ChunkDelta{Role: "assistant"},
			}},
		}
	}

	return nil
}

func (o *OpenAIResponsesAdapter) handleOutputItemAdded(state *responsesStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var evt dto.OutputItemAddedEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return fmt.Errorf("unmarshal response.output_item.added: %w", err)
	}

	if evt.Item.Type == "function_call" {
		idx := state.toolCallIndex
		state.itemIndexToToolIndex[evt.OutputIndex] = idx
		state.itemIndexToCallID[evt.OutputIndex] = evt.Item.CallID
		state.toolCallIndex++

		chunks <- &dto.ChatCompletionChunk{
			ID:     state.responseID,
			Object: "chat.completion.chunk",
			Model:  state.model,
			Choices: []dto.ChunkChoice{{
				Index: 0,
				Delta: dto.ChunkDelta{
					ToolCalls: []dto.ToolCall{{
						Index: intPtr(idx),
						ID:    evt.Item.CallID,
						Type:  "function",
						Function: dto.FunctionCall{
							Name:      evt.Item.Name,
							Arguments: "",
						},
					}},
				},
			}},
		}
	}

	return nil
}

func (o *OpenAIResponsesAdapter) handleOutputTextDelta(state *responsesStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var evt dto.OutputTextDeltaEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return fmt.Errorf("unmarshal response.output_text.delta: %w", err)
	}

	chunks <- &dto.ChatCompletionChunk{
		ID:     state.responseID,
		Object: "chat.completion.chunk",
		Model:  state.model,
		Choices: []dto.ChunkChoice{{
			Index: 0,
			Delta: dto.ChunkDelta{Content: evt.Delta},
		}},
	}

	return nil
}

func (o *OpenAIResponsesAdapter) handleFunctionCallArgsDelta(state *responsesStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var evt dto.FunctionCallArgsDeltaEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return fmt.Errorf("unmarshal response.function_call_arguments.delta: %w", err)
	}

	toolIdx := state.itemIndexToToolIndex[evt.OutputIndex]
	callID := state.itemIndexToCallID[evt.OutputIndex]

	chunks <- &dto.ChatCompletionChunk{
		ID:     state.responseID,
		Object: "chat.completion.chunk",
		Model:  state.model,
		Choices: []dto.ChunkChoice{{
			Index: 0,
			Delta: dto.ChunkDelta{
				ToolCalls: []dto.ToolCall{{
					Index: intPtr(toolIdx),
					ID:    callID,
					Function: dto.FunctionCall{
						Arguments: evt.Delta,
					},
				}},
			},
		}},
	}

	return nil
}

func (o *OpenAIResponsesAdapter) handleReasoningSummaryDelta(state *responsesStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var evt dto.ReasoningSummaryDeltaEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return fmt.Errorf("unmarshal response.reasoning_summary_text.delta: %w", err)
	}

	chunks <- &dto.ChatCompletionChunk{
		ID:     state.responseID,
		Object: "chat.completion.chunk",
		Model:  state.model,
		Choices: []dto.ChunkChoice{{
			Index: 0,
			Delta: dto.ChunkDelta{Thinking: evt.Delta},
		}},
	}

	return nil
}

func (o *OpenAIResponsesAdapter) handleResponseCompleted(state *responsesStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var evt dto.ResponseCompletedEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return fmt.Errorf("unmarshal response.completed: %w", err)
	}

	finishReason := "stop"

	// Check output items for function_call type → finish_reason = "tool_calls"
	if len(evt.Response.Output) > 0 {
		var outputItems []dto.ResponseOutputItem
		if err := json.Unmarshal(evt.Response.Output, &outputItems); err == nil {
			for _, item := range outputItems {
				if item.Type == "function_call" {
					finishReason = "tool_calls"
					break
				}
			}
		}
	}

	if evt.Response.Status == "incomplete" {
		finishReason = "length"
	}

	var usage *dto.Usage
	if evt.Response.Usage != nil {
		usage = &dto.Usage{
			PromptTokens:     evt.Response.Usage.InputTokens,
			CompletionTokens: evt.Response.Usage.OutputTokens,
			TotalTokens:      evt.Response.Usage.InputTokens + evt.Response.Usage.OutputTokens,
		}
		if evt.Response.Usage.InputTokensDetails != nil && evt.Response.Usage.InputTokensDetails.CachedTokens > 0 {
			usage.PromptTokensDetails = &dto.PromptTokensDetails{
				CachedTokens: evt.Response.Usage.InputTokensDetails.CachedTokens,
			}
		}
	}

	chunks <- &dto.ChatCompletionChunk{
		ID:     state.responseID,
		Object: "chat.completion.chunk",
		Model:  state.model,
		Choices: []dto.ChunkChoice{{
			Index:        0,
			Delta:        dto.ChunkDelta{},
			FinishReason: &finishReason,
		}},
		Usage: usage,
	}

	chunks <- nil // signals [DONE]
	return nil
}

func (o *OpenAIResponsesAdapter) handleResponseFailed(state *responsesStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var evt dto.ResponseErrorEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return fmt.Errorf("unmarshal response.failed: %w", err)
	}
	return fmt.Errorf("openai error: %s - %s", evt.Code, evt.Message)
}

func (o *OpenAIResponsesAdapter) handleResponseIncomplete(state *responsesStreamState, chunks chan<- *dto.ChatCompletionChunk) error {
	finishReason := "length"
	chunks <- &dto.ChatCompletionChunk{
		ID:     state.responseID,
		Object: "chat.completion.chunk",
		Model:  state.model,
		Choices: []dto.ChunkChoice{{
			Index:        0,
			Delta:        dto.ChunkDelta{},
			FinishReason: &finishReason,
		}},
	}
	chunks <- nil
	return nil
}
