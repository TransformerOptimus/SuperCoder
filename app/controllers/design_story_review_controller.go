package controllers

import (
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"github.com/gin-gonic/gin"
	"net/http"
)

type DesignStoryReviewController struct {
	designStoryService *services.DesignStoryReviewService
	storyService       *services.StoryService
}

func NewDesignStoryReviewController(
	designStoryService *services.DesignStoryReviewService,
	storyService *services.StoryService,
) *DesignStoryReviewController {
	return &DesignStoryReviewController{
		designStoryService: designStoryService,
		storyService:       storyService,
	}
}

func (ctrl *DesignStoryReviewController) CreateCommentForDesignStory(c *gin.Context) {
	var createCommentRequest request.CreateDesignStoryCommentRequest
	if err := c.ShouldBindJSON(&createCommentRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := ctrl.designStoryService.CreateComment(createCommentRequest.StoryID, createCommentRequest.Comment)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "OK"})
	return
}
