package controllers

import "github.com/gin-gonic/gin"

type HealthController struct {
}

func (h *HealthController) Health(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "healthy",
	})
}

func NewHealthController() *HealthController {
	return &HealthController{}
}
