package controllers

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	apicontext "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/api/context"
	coreconfig "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/core"
)

type HealthController struct {
	logger        *zap.Logger
	serviceConfig *coreconfig.ServiceConfig
}

func (h *HealthController) Health(c *apicontext.Context) {
	c.Ok(gin.H{
		"message": "healthy",
		"service": h.serviceConfig.Name(),
	})
}

func NewHealthController(logger *zap.Logger, serviceConfig *coreconfig.ServiceConfig) *HealthController {
	return &HealthController{
		logger:        logger.Named("controllers.HealthController"),
		serviceConfig: serviceConfig,
	}
}
