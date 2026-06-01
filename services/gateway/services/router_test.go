package services

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

type mockAdapter struct {
	name string
}

func (m *mockAdapter) TranslateRequest(req *dto.ChatCompletionRequest, apiKey string) (string, map[string]string, []byte, error) {
	return "", nil, nil, nil
}

func (m *mockAdapter) TranslateStream(ctx context.Context, body io.Reader, chunks chan<- *dto.ChatCompletionChunk) error {
	return nil
}

func (m *mockAdapter) MatchesModel(model string) bool {
	return false
}

func (m *mockAdapter) Name() string {
	return m.name
}

func (m *mockAdapter) ConfiguredAPIKey() string {
	return ""
}

func TestRouterPrefixMatch(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger, "openai")

	anthropic := &mockAdapter{name: "anthropic"}
	openai := &mockAdapter{name: "openai"}

	router.RegisterProvider("anthropic", anthropic, []string{"claude-"})
	router.RegisterProvider("openai", openai, []string{"gpt-5", "o3", "o4"})

	tests := []struct {
		model    string
		expected string
	}{
		{"claude-sonnet-4-6", "anthropic"},
		{"claude-opus-4-6", "anthropic"},
		{"claude-haiku-4-5-20251001", "anthropic"},
		{"gpt-5.4", "openai"},
		{"gpt-5.4-mini", "openai"},
		{"o3", "openai"},
		{"o3-mini", "openai"},
		{"o4-mini", "openai"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			adapter, err := router.Route(tt.model, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, adapter.Name())
		})
	}
}

func TestRouterDefaultFallback(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger, "openai")

	openai := &mockAdapter{name: "openai"}
	router.RegisterProvider("openai", openai, []string{"gpt-5"})

	adapter, err := router.Route("unknown-model", "")
	require.NoError(t, err)
	assert.Equal(t, "openai", adapter.Name())
}

func TestRouterProviderOverride(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger, "openai")

	anthropic := &mockAdapter{name: "anthropic"}
	openai := &mockAdapter{name: "openai"}

	router.RegisterProvider("anthropic", anthropic, []string{"claude-"})
	router.RegisterProvider("openai", openai, []string{"gpt-5"})

	// Override: send a claude model but force openai provider
	adapter, err := router.Route("claude-sonnet-4-6", "openai")
	require.NoError(t, err)
	assert.Equal(t, "openai", adapter.Name())
}

func TestRouterUnknownOverride(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger, "openai")

	openai := &mockAdapter{name: "openai"}
	router.RegisterProvider("openai", openai, []string{"gpt-5"})

	_, err := router.Route("gpt-5.4", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider override")
}

func TestRouterNoProviders(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger, "nonexistent")

	_, err := router.Route("any-model", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider found")
}

func TestRouterLongestPrefixMatch(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger, "default")

	short := &mockAdapter{name: "short"}
	long := &mockAdapter{name: "long"}
	def := &mockAdapter{name: "default"}

	router.RegisterProvider("short", short, []string{"gpt-"})
	router.RegisterProvider("long", long, []string{"gpt-5"})
	router.RegisterProvider("default", def, []string{})

	adapter, err := router.Route("gpt-5.4", "")
	require.NoError(t, err)
	assert.Equal(t, "long", adapter.Name(), "should match longer prefix 'gpt-5' over 'gpt-'")

	adapter, err = router.Route("gpt-4o", "")
	require.NoError(t, err)
	assert.Equal(t, "short", adapter.Name(), "should match 'gpt-' for gpt-4o")
}
