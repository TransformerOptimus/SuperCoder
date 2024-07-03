package controllers

import (
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
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
	orgId, err := c.userService.FetchOrganisationIDByUserID(uint(userID.(int)))
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	err = c.llmAPIKeyService.CreateOrUpdateLLMAPIKey(orgId, createLLMAPIKey.LLMModel, createLLMAPIKey.LLMAPIKey)
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"message": "LLM API Key created successfully"})
}

func (c *LLMAPIKeyController) FetchAllLLMAPIKeyByOrganisationID(context *gin.Context) {
	userID, exists := context.Get("user_id")
	if !exists {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	organisationID := context.Param("organisation_id")
	organisationIDInt, err := strconv.Atoi(organisationID)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organisation ID"})
		return
	}
	userIDInt, ok := userID.(int)
	if !ok {
		context.JSON(http.StatusBadRequest, gin.H{"error": "User ID is not of type int"})
		return
	}
	var organisationIdByUserID uint
	organisationIdByUserID, err = c.userService.FetchOrganisationIDByUserID(uint(userIDInt))
	if organisationIDInt != int(organisationIdByUserID) {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "User does not have access to this organisation"})
		return
	}
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organisation ID"})
		return
	}

	output, err := c.llmAPIKeyService.GetAllLLMAPIKeyByOrganisationID(uint(organisationIDInt))
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
