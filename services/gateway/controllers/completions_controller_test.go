package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/gateway/services"
	"github.com/TransformerOptimus/SuperCoder/services/gateway/services/impl"
)

func setupRouter(t *testing.T) (*gin.Engine, *services.Router, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Mock Anthropic server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"model\":\"claude-test\",\"usage\":{\"input_tokens\":5}}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "event: content_block_start\ndata: {\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "event: content_block_stop\ndata: {\"index\":0}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "event: message_delta\ndata: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "event: message_stop\ndata: {}\n\n")
		flusher.Flush()
	}))

	logger := zap.NewNop()
	router := services.NewRouter(logger, "anthropic")

	adapter := impl.NewAnthropicAdapter(mockServer.URL, "2023-06-01", 8192, "", logger)
	router.RegisterProvider("anthropic", adapter, []string{"claude-"})

	ctrl := NewCompletionsController(router, logger)
	r := gin.New()
	r.POST("/v1/chat/completions", ctrl.HandleCompletion)

	return r, router, mockServer
}

func TestHandleCompletion_Success(t *testing.T) {
	r, _, mockServer := setupRouter(t)
	defer mockServer.Close()

	body := `{"model":"claude-test","messages":[{"role":"user","content":"Hi"}],"stream":true,"max_completion_tokens":50}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

	respBody := w.Body.String()
	assert.Contains(t, respBody, "data: ")
	assert.Contains(t, respBody, "data: [DONE]")

	// Parse first chunk — should have role
	lines := strings.Split(respBody, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "data: {") {
			var chunk dto.ChatCompletionChunk
			err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk)
			require.NoError(t, err)
			assert.Equal(t, "msg_test", chunk.ID)
			assert.Equal(t, "chat.completion.chunk", chunk.Object)
			break
		}
	}
}

func TestHandleCompletion_InvalidJSON(t *testing.T) {
	r, _, mockServer := setupRouter(t)
	defer mockServer.Close()

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errResp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "invalid_request_error", errResp.Error.Type)
}

func TestHandleCompletion_MissingModel(t *testing.T) {
	r, _, mockServer := setupRouter(t)
	defer mockServer.Close()

	body := `{"messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errResp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Contains(t, errResp.Error.Message, "model is required")
}

func TestHandleCompletion_UnknownProvider(t *testing.T) {
	r, _, mockServer := setupRouter(t)
	defer mockServer.Close()

	body := `{"model":"claude-test","messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Provider", "nonexistent")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errResp dto.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Contains(t, errResp.Error.Message, "unknown provider override")
}

func TestHandleCompletion_ProviderError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"invalid api key","type":"authentication_error"}}`)
	}))
	defer errorServer.Close()

	logger := zap.NewNop()
	router := services.NewRouter(logger, "anthropic")
	adapter := impl.NewAnthropicAdapter(errorServer.URL, "2023-06-01", 8192, "", logger)
	router.RegisterProvider("anthropic", adapter, []string{"claude-"})

	ctrl := NewCompletionsController(router, logger)
	r := gin.New()
	r.POST("/v1/chat/completions", ctrl.HandleCompletion)

	body := `{"model":"claude-test","messages":[{"role":"user","content":"Hi"}],"stream":true}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer bad-key")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestExtractAPIKey(t *testing.T) {
	assert.Equal(t, "sk-123", extractAPIKey("Bearer sk-123"))
	assert.Equal(t, "raw-key", extractAPIKey("raw-key"))
	assert.Equal(t, "", extractAPIKey(""))
}
