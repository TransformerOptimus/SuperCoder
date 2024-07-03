package controllers

import (
	"ai-developer/app/services"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type ActivityLogController struct {
	Service *services.ActivityLogService
}

func NewActivityLogController(service *services.ActivityLogService) *ActivityLogController {
	return &ActivityLogController{Service: service}
}

func (ctrl *ActivityLogController) GetActivityLogsByStoryID(c *gin.Context) {
	storyIDParam := c.Param("story_id")
	storyID, err := strconv.Atoi(storyIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid story ID"})
		return
	}

	activityLogs, err := ctrl.Service.GetActivityLogsByStoryID(uint(storyID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch activity logs"})
		return
	}

	c.JSON(http.StatusOK, activityLogs)
}
