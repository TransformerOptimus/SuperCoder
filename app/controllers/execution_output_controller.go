package controllers

import (
	"ai-developer/app/services"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type ExecutionOutputController struct {
	Service *services.ExecutionOutputService
}

func NewExecutionOutputController(service *services.ExecutionOutputService) *ExecutionOutputController {
	return &ExecutionOutputController{Service: service}
}

func (ctrl *ExecutionOutputController) GetExecutionOutputsByStoryID(c *gin.Context) {
	storyIDParam := c.Param("story_id")
	storyID, err := strconv.Atoi(storyIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid story ID"})
		return
	}

	executionOutputs, err := ctrl.Service.GetExecutionOutputsByStoryID(uint(storyID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, executionOutputs)
}
