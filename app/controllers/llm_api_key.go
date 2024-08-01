package controllers

import (
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"github.com/gin-gonic/gin"
	"net/http"
)

type LLMAPIKeyController struct {
	llmAPIKeyService *services.LLMAPIKeyService
	userService      *services.UserService
}

func (c *LLMAPIKeyController) CreateLLMAPIKey(context *gin.Context) {
	var createLLMAPIKey request.CreateLLMAPIKeyRequest
	if err := context.ShouldBindJSON(&createLLMAPIKey); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := context.Get("user_id")
	if !exists {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	orgId, err := c.userService.FetchOrganisationIDByUserID(userID.(uint))
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, apiKey := range createLLMAPIKey.APIKeys {
		if apiKey.LLMAPIKey == nil {
			err = c.llmAPIKeyService.CreateOrUpdateLLMAPIKey(orgId, apiKey.LLMModel, "")
		} else {
			err = c.llmAPIKeyService.CreateOrUpdateLLMAPIKey(orgId, apiKey.LLMModel, *apiKey.LLMAPIKey)
		}
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	context.JSON(http.StatusOK, gin.H{"message": "LLM API Keys created successfully"})
}

func (c *LLMAPIKeyController) FetchAllLLMAPIKeyByOrganisationID(context *gin.Context) {
	userID, exists := context.Get("user_id")
	if !exists {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	userIDInt, ok := userID.(uint)
	if !ok {
		context.JSON(http.StatusBadRequest, gin.H{"error": "User ID is not of type int"})
		return
	}
	organisationIdByUserID, err := c.userService.FetchOrganisationIDByUserID(uint(userIDInt))
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organisation ID"})
		return
	}

	output, err := c.llmAPIKeyService.GetAllLLMAPIKeyByOrganisationID(organisationIdByUserID)
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	context.JSON(http.StatusOK, output)
}

func NewLLMAPIKeyController(llmAPIKeyService *services.LLMAPIKeyService, userService *services.UserService) *LLMAPIKeyController {
	return &LLMAPIKeyController{
		llmAPIKeyService: llmAPIKeyService,
		userService:      userService,
	}
}
