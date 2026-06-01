package services

import (
	"context"
	"io"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

// ProviderAdapter translates between OpenAI Chat Completions format and a specific LLM provider.
type ProviderAdapter interface {
	TranslateRequest(req *dto.ChatCompletionRequest, apiKey string) (url string, headers map[string]string, body []byte, err error)
	TranslateStream(ctx context.Context, providerBody io.Reader, chunks chan<- *dto.ChatCompletionChunk) error
	MatchesModel(model string) bool
	Name() string
	ConfiguredAPIKey() string
}
