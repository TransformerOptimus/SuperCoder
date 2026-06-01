package impl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

// OpenAICompatAdapter is a pure passthrough for any OpenAI-compatible endpoint
// (vLLM, Ollama, TGI, etc). No translation — just proxies Chat Completions requests.
type OpenAICompatAdapter struct {
	baseURL  string
	name     string
	apiKey   string
	prefixes []string
	logger   *zap.Logger
}

func NewOpenAICompatAdapter(name, baseURL, apiKey string, prefixes []string, logger *zap.Logger) *OpenAICompatAdapter {
	return &OpenAICompatAdapter{
		baseURL:  strings.TrimRight(baseURL, "/"),
		name:     name,
		apiKey:   apiKey,
		prefixes: prefixes,
		logger:   logger.Named("gateway.openai-compat." + name),
	}
}

func (o *OpenAICompatAdapter) ConfiguredAPIKey() string {
	return o.apiKey
}

func (o *OpenAICompatAdapter) Name() string {
	return o.name
}

func (o *OpenAICompatAdapter) MatchesModel(model string) bool {
	for _, p := range o.prefixes {
		if strings.HasPrefix(model, p) {
			return true
		}
	}
	return false
}

func (o *OpenAICompatAdapter) TranslateRequest(req *dto.ChatCompletionRequest, apiKey string) (string, map[string]string, []byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := o.baseURL + "/chat/completions"
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}

	return url, headers, body, nil
}

// TranslateStream parses standard OpenAI SSE and forwards chunks unchanged.
func (o *OpenAICompatAdapter) TranslateStream(ctx context.Context, providerBody io.Reader, chunks chan<- *dto.ChatCompletionChunk) error {
	defer close(chunks)

	scanner := bufio.NewScanner(providerBody)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			chunks <- nil
			return nil
		}

		var chunk dto.ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			o.logger.Warn("failed to parse chunk", zap.Error(err))
			continue
		}

		chunks <- &chunk
	}

	return scanner.Err()
}
