package controllers

import (
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"github.com/gin-gonic/gin"
	"net/http"
)

type PullRequestCommentsController struct {
	pullRequestCommentService *services.PullRequestCommentsService
}

func NewPullRequestCommentController(pullRequestCommentService *services.PullRequestCommentsService) *PullRequestCommentsController {
	return &PullRequestCommentsController{
		pullRequestCommentService: pullRequestCommentService,
	}
}

func (ctrl *PullRequestCommentsController) CreateCommentForPrID(c *gin.Context) {
	var createCommentRequest request.CreateCommentRequest
	if err := c.ShouldBindJSON(&createCommentRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := ctrl.pullRequestCommentService.CreateComment(createCommentRequest.PullRequestID, createCommentRequest.Comment)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "OK"})
	return
}
