package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSSEStream_BasicEvent(t *testing.T) {
	input := `event: message_start
data: {"type":"message_start"}

`
	var events []SSEEvent
	err := ParseSSEStream(context.Background(), strings.NewReader(input), func(evt SSEEvent) error {
		events = append(events, evt)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "message_start", events[0].Event)
	assert.Equal(t, `{"type":"message_start"}`, string(events[0].Data))
}

func TestParseSSEStream_MultipleEvents(t *testing.T) {
	input := `event: first
data: {"n":1}

event: second
data: {"n":2}

`
	var events []SSEEvent
	err := ParseSSEStream(context.Background(), strings.NewReader(input), func(evt SSEEvent) error {
		events = append(events, evt)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "first", events[0].Event)
	assert.Equal(t, "second", events[1].Event)
}

func TestParseSSEStream_DataWithoutEvent(t *testing.T) {
	input := `data: {"plain":true}

`
	var events []SSEEvent
	err := ParseSSEStream(context.Background(), strings.NewReader(input), func(evt SSEEvent) error {
		events = append(events, evt)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "", events[0].Event)
	assert.Equal(t, `{"plain":true}`, string(events[0].Data))
}

func TestParseSSEStream_MultiLineData(t *testing.T) {
	input := `event: multi
data: line1
data: line2

`
	var events []SSEEvent
	err := ParseSSEStream(context.Background(), strings.NewReader(input), func(evt SSEEvent) error {
		events = append(events, evt)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "line1\nline2", string(events[0].Data))
}

func TestParseSSEStream_CommentsIgnored(t *testing.T) {
	input := `: this is a comment
event: test
data: {"ok":true}

`
	var events []SSEEvent
	err := ParseSSEStream(context.Background(), strings.NewReader(input), func(evt SSEEvent) error {
		events = append(events, evt)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "test", events[0].Event)
}

func TestParseSSEStream_EventWithoutData(t *testing.T) {
	// Event type set but no data lines — should not fire handler
	input := `event: empty

event: has_data
data: {"ok":true}

`
	var events []SSEEvent
	err := ParseSSEStream(context.Background(), strings.NewReader(input), func(evt SSEEvent) error {
		events = append(events, evt)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "has_data", events[0].Event)
}

func TestParseSSEStream_HandlerError(t *testing.T) {
	input := `event: test
data: {"n":1}

event: test
data: {"n":2}

`
	handlerErr := errors.New("stop processing")
	var count int
	err := ParseSSEStream(context.Background(), strings.NewReader(input), func(evt SSEEvent) error {
		count++
		return handlerErr
	})
	assert.ErrorIs(t, err, handlerErr)
	assert.Equal(t, 1, count, "should stop after first handler error")
}

func TestParseSSEStream_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	input := `event: test
data: {"n":1}

`
	err := ParseSSEStream(ctx, strings.NewReader(input), func(evt SSEEvent) error {
		t.Fatal("handler should not be called after context cancellation")
		return nil
	})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestParseSSEStream_TrailingEventWithoutBlankLine(t *testing.T) {
	// EOF without trailing blank line — should still process the event
	input := `event: trailing
data: {"last":true}`

	var events []SSEEvent
	err := ParseSSEStream(context.Background(), strings.NewReader(input), func(evt SSEEvent) error {
		events = append(events, evt)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "trailing", events[0].Event)
}

func TestParseSSEStream_EmptyInput(t *testing.T) {
	var events []SSEEvent
	err := ParseSSEStream(context.Background(), strings.NewReader(""), func(evt SSEEvent) error {
		events = append(events, evt)
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestParseSSEStream_DataSpaceHandling(t *testing.T) {
	// "data: value" and "data:value" should both work
	input := `data: with space

data:no space

`
	var events []SSEEvent
	err := ParseSSEStream(context.Background(), strings.NewReader(input), func(evt SSEEvent) error {
		events = append(events, evt)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "with space", string(events[0].Data))
	assert.Equal(t, "no space", string(events[1].Data))
}
