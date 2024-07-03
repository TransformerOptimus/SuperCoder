package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type HealthController struct {
}

func NewHealth() *HealthController {
	return &HealthController{}
}

func (h *HealthController) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "OK"})
}
