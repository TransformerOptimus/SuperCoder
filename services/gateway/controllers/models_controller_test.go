package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/config"
	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

type stubGatewayConfig struct {
	models []config.ModelEntry
}

func (s *stubGatewayConfig) DefaultProvider() string           { return "" }
func (s *stubGatewayConfig) Providers() []config.ProviderEntry { return nil }
func (s *stubGatewayConfig) Models() []config.ModelEntry       { return s.models }

func TestListModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &stubGatewayConfig{
		models: []config.ModelEntry{
			{ID: "claude-opus-4-6", DisplayName: "Claude Opus 4.6", Provider: "anthropic", ContextWindow: 1000000},
			{ID: "gpt-5", DisplayName: "GPT-5", Provider: "openai", ContextWindow: 400000},
		},
	}

	ctrl := NewModelsController(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	ctrl.ListModels(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.ModelsResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.Len(t, resp.Models, 2)
	assert.Equal(t, "claude-opus-4-6", resp.Models[0].ID)
	assert.Equal(t, "Claude Opus 4.6", resp.Models[0].DisplayName)
	assert.Equal(t, "anthropic", resp.Models[0].Provider)
	assert.Equal(t, 1000000, resp.Models[0].ContextWindow)
	assert.Equal(t, "gpt-5", resp.Models[1].ID)
	assert.Equal(t, 400000, resp.Models[1].ContextWindow)
}

func TestListModelsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctrl := NewModelsController(&stubGatewayConfig{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	ctrl.ListModels(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.ModelsResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Empty(t, resp.Models)
}
