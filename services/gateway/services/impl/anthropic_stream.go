package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/gateway/services"
)

type anthropicStreamState struct {
	messageID           string
	model               string
	inputTokens         int
	cacheCreationTokens int
	cacheReadTokens     int
	currentBlockType    string
	toolCallIndex       int
	blockToToolIndex    map[int]int
	blockToToolID       map[int]string
	hasEmittedRole      bool
}

// TranslateStream reads Anthropic SSE events and sends Chat Completions chunks.
func (a *AnthropicAdapter) TranslateStream(ctx context.Context, providerBody io.Reader, chunks chan<- *dto.ChatCompletionChunk) error {
	defer close(chunks)

	state := &anthropicStreamState{
		blockToToolIndex: make(map[int]int),
		blockToToolID:    make(map[int]string),
	}

	return services.ParseSSEStream(ctx, providerBody, func(evt services.SSEEvent) error {
		return a.handleAnthropicEvent(state, evt, chunks)
	})
}

func (a *AnthropicAdapter) handleAnthropicEvent(state *anthropicStreamState, evt services.SSEEvent, chunks chan<- *dto.ChatCompletionChunk) error {
	handler := func() error {
		switch evt.Event {
		case "message_start":
			return a.handleMessageStart(state, evt.Data, chunks)
		case "content_block_start":
			return a.handleContentBlockStart(state, evt.Data, chunks)
		case "content_block_delta":
			return a.handleContentBlockDelta(state, evt.Data, chunks)
		case "content_block_stop":
			state.currentBlockType = ""
			return nil
		case "message_delta":
			return a.handleMessageDelta(state, evt.Data, chunks)
		case "message_stop":
			chunks <- nil // signals [DONE]
			return nil
		case "ping":
			chunks <- &dto.ChatCompletionChunk{Object: "ping"}
			return nil
		case "error":
			return a.handleError(state, evt.Data, chunks)
		default:
			return nil
		}
	}

	if err := handler(); err != nil {
		a.logger.Error("stream event handling failed",
			zap.String("event_type", evt.Event),
			zap.Int("payload_bytes", len(evt.Data)),
			zap.Error(err),
		)
		return err
	}
	return nil
}

func (a *AnthropicAdapter) handleMessageStart(state *anthropicStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var msg dto.MessageStartData
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("unmarshal message_start: %w", err)
	}

	state.messageID = msg.Message.ID
	state.model = msg.Message.Model
	if msg.Message.Usage != nil {
		state.inputTokens = msg.Message.Usage.InputTokens
		state.cacheCreationTokens = msg.Message.Usage.CacheCreationInputTokens
		state.cacheReadTokens = msg.Message.Usage.CacheReadInputTokens
	}

	if !state.hasEmittedRole {
		state.hasEmittedRole = true
		chunks <- &dto.ChatCompletionChunk{
			ID:     state.messageID,
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

func (a *AnthropicAdapter) handleContentBlockStart(state *anthropicStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var block dto.ContentBlockStartData
	if err := json.Unmarshal(data, &block); err != nil {
		return fmt.Errorf("unmarshal content_block_start: %w", err)
	}

	state.currentBlockType = block.ContentBlock.Type

	switch block.ContentBlock.Type {
	case "tool_use":
		idx := state.toolCallIndex
		state.blockToToolIndex[block.Index] = idx
		state.blockToToolID[block.Index] = block.ContentBlock.ID
		state.toolCallIndex++

		chunks <- &dto.ChatCompletionChunk{
			ID:     state.messageID,
			Object: "chat.completion.chunk",
			Model:  state.model,
			Choices: []dto.ChunkChoice{{
				Index: 0,
				Delta: dto.ChunkDelta{
					ToolCalls: []dto.ToolCall{{
						Index: intPtr(idx),
						ID:    block.ContentBlock.ID,
						Type:  "function",
						Function: dto.FunctionCall{
							Name:      block.ContentBlock.Name,
							Arguments: "",
						},
					}},
				},
			}},
		}
	case "thinking":
		// Record type, deltas will follow
	case "text":
		// Record type, deltas will follow
	}

	return nil
}

func (a *AnthropicAdapter) handleContentBlockDelta(state *anthropicStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var delta dto.ContentBlockDeltaData
	if err := json.Unmarshal(data, &delta); err != nil {
		return fmt.Errorf("unmarshal content_block_delta: %w", err)
	}

	switch delta.Delta.Type {
	case "text_delta":
		chunks <- &dto.ChatCompletionChunk{
			ID:     state.messageID,
			Object: "chat.completion.chunk",
			Model:  state.model,
			Choices: []dto.ChunkChoice{{
				Index: 0,
				Delta: dto.ChunkDelta{Content: delta.Delta.Text},
			}},
		}

	case "input_json_delta":
		toolIdx, ok := state.blockToToolIndex[delta.Index]
		if !ok {
			toolIdx = 0
		}
		toolID := state.blockToToolID[delta.Index]
		chunks <- &dto.ChatCompletionChunk{
			ID:     state.messageID,
			Object: "chat.completion.chunk",
			Model:  state.model,
			Choices: []dto.ChunkChoice{{
				Index: 0,
				Delta: dto.ChunkDelta{
					ToolCalls: []dto.ToolCall{{
						Index: intPtr(toolIdx),
						ID:    toolID,
						Function: dto.FunctionCall{
							Arguments: delta.Delta.PartialJSON,
						},
					}},
				},
			}},
		}

	case "thinking_delta":
		chunks <- &dto.ChatCompletionChunk{
			ID:     state.messageID,
			Object: "chat.completion.chunk",
			Model:  state.model,
			Choices: []dto.ChunkChoice{{
				Index: 0,
				Delta: dto.ChunkDelta{Thinking: delta.Delta.Thinking},
			}},
		}

	case "signature_delta":
		// Drop
	}

	return nil
}

func (a *AnthropicAdapter) handleMessageDelta(state *anthropicStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var msgDelta dto.MessageDeltaData
	if err := json.Unmarshal(data, &msgDelta); err != nil {
		return fmt.Errorf("unmarshal message_delta: %w", err)
	}

	finishReason := mapAnthropicStopReason(msgDelta.Delta.StopReason)

	var usage *dto.Usage
	if msgDelta.Usage != nil {
		// Prefer non-zero delta values over message_start values.
		if msgDelta.Usage.CacheCreationInputTokens > 0 {
			state.cacheCreationTokens = msgDelta.Usage.CacheCreationInputTokens
		}
		if msgDelta.Usage.CacheReadInputTokens > 0 {
			state.cacheReadTokens = msgDelta.Usage.CacheReadInputTokens
		}
		// Anthropic's input_tokens excludes cached tokens. Include cache_read
		// + cache_creation in prompt/total so all providers report consistent
		// totals (OpenAI already includes cached in prompt_tokens).
		totalInput := state.inputTokens + state.cacheReadTokens + state.cacheCreationTokens
		usage = &dto.Usage{
			PromptTokens:     totalInput,
			CompletionTokens: msgDelta.Usage.OutputTokens,
			TotalTokens:      totalInput + msgDelta.Usage.OutputTokens,
		}
		if state.cacheCreationTokens > 0 || state.cacheReadTokens > 0 {
			usage.PromptTokensDetails = &dto.PromptTokensDetails{
				CachedTokens:        state.cacheReadTokens,
				CacheCreationTokens: state.cacheCreationTokens,
			}
		}
	}

	chunks <- &dto.ChatCompletionChunk{
		ID:     state.messageID,
		Object: "chat.completion.chunk",
		Model:  state.model,
		Choices: []dto.ChunkChoice{{
			Index:        0,
			Delta:        dto.ChunkDelta{},
			FinishReason: &finishReason,
		}},
		Usage: usage,
	}

	return nil
}

func (a *AnthropicAdapter) handleError(state *anthropicStreamState, data []byte, chunks chan<- *dto.ChatCompletionChunk) error {
	var errData dto.AnthropicErrorData
	if err := json.Unmarshal(data, &errData); err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	return fmt.Errorf("anthropic error: %s - %s", errData.Error.Type, errData.Error.Message)
}

func intPtr(i int) *int { return &i }

func mapAnthropicStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	default:
		return "stop"
	}
}
