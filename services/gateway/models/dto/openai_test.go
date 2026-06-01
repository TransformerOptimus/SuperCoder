package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMessageContent_String(t *testing.T) {
	raw := json.RawMessage(`"Hello world"`)
	text, blocks, err := ParseMessageContent(raw)
	require.NoError(t, err)
	assert.Equal(t, "Hello world", text)
	assert.Nil(t, blocks)
}

func TestParseMessageContent_Array(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"Hello"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]`)
	text, blocks, err := ParseMessageContent(raw)
	require.NoError(t, err)
	assert.Empty(t, text)
	require.Len(t, blocks, 2)
	assert.Equal(t, "text", blocks[0].Type)
	assert.Equal(t, "Hello", blocks[0].Text)
	assert.Equal(t, "image_url", blocks[1].Type)
	require.NotNil(t, blocks[1].ImageURL)
	assert.Equal(t, "data:image/png;base64,abc", blocks[1].ImageURL.URL)
}

func TestParseMessageContent_Empty(t *testing.T) {
	text, blocks, err := ParseMessageContent(nil)
	require.NoError(t, err)
	assert.Empty(t, text)
	assert.Nil(t, blocks)
}

func TestParseMessageContent_EmptyRaw(t *testing.T) {
	text, blocks, err := ParseMessageContent(json.RawMessage{})
	require.NoError(t, err)
	assert.Empty(t, text)
	assert.Nil(t, blocks)
}

func TestParseMessageContent_Invalid(t *testing.T) {
	raw := json.RawMessage(`12345`)
	_, _, err := ParseMessageContent(raw)
	assert.Error(t, err)
}

func TestParseMessageContent_EmptyString(t *testing.T) {
	raw := json.RawMessage(`""`)
	text, blocks, err := ParseMessageContent(raw)
	require.NoError(t, err)
	assert.Equal(t, "", text)
	assert.Nil(t, blocks)
}

func TestParseMessageContent_EmptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)
	text, blocks, err := ParseMessageContent(raw)
	require.NoError(t, err)
	assert.Empty(t, text)
	assert.Empty(t, blocks)
}
