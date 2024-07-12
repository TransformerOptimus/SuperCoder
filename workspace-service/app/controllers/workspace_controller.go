package controllers

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"workspace-service/app/models/dto"
	"workspace-service/app/services"
)

type WorkspaceController struct {
	wsService services.WorkspaceService
	logger    *zap.Logger
}

func (wc *WorkspaceController) CreateWorkspace(c *gin.Context) {
	body := dto.CreateWorkspace{}
	if err := c.BindJSON(&body); err != nil {
		wc.logger.Error("Failed to bind json", zap.Error(err))
		c.AbortWithStatusJSON(400, gin.H{
			"error": "Bad Request",
		})
		return
	}
	wsDetails, err := wc.wsService.CreateWorkspace(body.WorkspaceId, *body.BackendTemplate, body.FrontendTemplate, body.RemoteURL, body.GitnessUserName, body.GitnessToken)
	if err != nil {
		c.AbortWithStatusJSON(
			500,
			gin.H{"error": "Internal Server Error"},
		)
		return
	}
	c.JSON(
		200,
		gin.H{"message": "success", "workspace": wsDetails},
	)
}

func (wc *WorkspaceController) CreateFrontendWorkspace(c *gin.Context) {
	nextjs := "nextjs"
	body := dto.CreateWorkspace{
		FrontendTemplate: &nextjs,
	}
	if err := c.BindJSON(&body); err != nil {
		wc.logger.Error("Failed to bind json", zap.Error(err))
		c.AbortWithStatusJSON(400, gin.H{
			"error": "Bad Request",
		})
		return
	}
	wsDetails, err := wc.wsService.CreateFrontendWorkspace(body.StoryHashId, body.WorkspaceId, *body.FrontendTemplate)
	if err != nil {
		c.AbortWithStatusJSON(
			500,
			gin.H{"error": "Internal Server Error"},
		)
		return
	}
	c.JSON(
		200,
		gin.H{"message": "success", "workspace": wsDetails},
	)
}

func (wc *WorkspaceController) DeleteWorkspace(c *gin.Context) {
	workspaceId := c.Param("workspaceId")
	err := wc.wsService.DeleteWorkspace(workspaceId)
	if err != nil {
		c.AbortWithStatusJSON(
			500,
			gin.H{"error": "Internal Server Error"},
		)
		return
	}
	c.JSON(
		200,
		gin.H{"message": "success"},
	)

}

func NewWorkspaceController(
	logger *zap.Logger,
	wsService services.WorkspaceService,
) *WorkspaceController {
	return &WorkspaceController{
		wsService: wsService,
		logger:    logger,
	}
}
