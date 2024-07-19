package controllers

import (
	"ai-developer/app/services/integrations"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
)

type GithubIntegrationController struct {
	githubIntegrationService *integrations.GithubIntegrationService
	logger                   *zap.Logger
}

func (gic *GithubIntegrationController) Authorize(c *gin.Context) {
	userId, _ := c.Get("user_id")
	gic.logger.Debug(
		"Authorizing github integration",
		zap.Any("user_id", userId),
	)
	authCodeUrl := gic.githubIntegrationService.GetRedirectUrl(uint64(userId.(int)))
	c.Redirect(http.StatusTemporaryRedirect, authCodeUrl)
}

func (gic *GithubIntegrationController) CheckIfIntegrationExists(c *gin.Context) {
	userId, _ := c.Get("user_id")
	hasIntegration, err := gic.githubIntegrationService.HasGithubIntegration(uint64(userId.(int)))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"integrated": hasIntegration})
}

func (gic *GithubIntegrationController) GetRepositories(c *gin.Context) {
	userId, _ := c.Get("user_id")
	repositories, err := gic.githubIntegrationService.GetRepositories(uint64(userId.(int)))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response := make([]map[string]interface{}, 0)
	for _, repo := range repositories {
		response = append(response, map[string]interface{}{
			"id":   repo.GetID(),
			"url":  repo.GetCloneURL(),
			"name": repo.GetFullName(),
		})
	}
	c.JSON(http.StatusOK, gin.H{"repositories": response})
}

func (gic *GithubIntegrationController) HandleCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	gic.logger.Debug(
		"Handling github integration callback",
		zap.String("code", code),
		zap.String("state", state),
	)

	err := gic.githubIntegrationService.GenerateAndSaveAccessToken(code, state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else {
		c.JSON(http.StatusOK, gin.H{"message": "Integration successful"})
		return
	}
}

func NewGithubIntegrationController(
	githubIntegrationService *integrations.GithubIntegrationService,
	logger *zap.Logger,
) *GithubIntegrationController {
	return &GithubIntegrationController{
		githubIntegrationService: githubIntegrationService,
		logger:                   logger,
	}
}
