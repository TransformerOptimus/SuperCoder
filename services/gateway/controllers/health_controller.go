package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GatewayHealthController handles GET /health for the gateway.
type GatewayHealthController struct{}

func NewGatewayHealthController() *GatewayHealthController {
	return &GatewayHealthController{}
}

func (h *GatewayHealthController) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "healthy",
		"service": "gateway",
	})
}
