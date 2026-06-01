package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/config"
	"github.com/TransformerOptimus/SuperCoder/services/gateway/models/dto"
)

// ModelsController handles GET /v1/models.
type ModelsController struct {
	config config.GatewayConfig
}

func NewModelsController(cfg config.GatewayConfig) *ModelsController {
	return &ModelsController{config: cfg}
}

func (ctrl *ModelsController) ListModels(c *gin.Context) {
	entries := ctrl.config.Models()
	models := make([]dto.ModelInfo, len(entries))
	for i, e := range entries {
		models[i] = dto.ModelInfo{
			ID:             e.ID,
			DisplayName:    e.DisplayName,
			Provider:       e.Provider,
			ContextWindow:  e.ContextWindow,
			SupportsImages: e.SupportsImages,
		}
	}
	c.JSON(http.StatusOK, dto.ModelsResponse{Models: models})
}
