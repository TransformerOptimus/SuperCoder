package controllers

import (
	"ai-developer/app/models"
	"ai-developer/app/types/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
)

type UserController struct {
	logger *zap.Logger
}

func (controller *UserController) GetUserDetails(c *gin.Context) {
	userInterface, exists := c.Get("user")
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	user, exists := userInterface.(*models.User)
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userDetails := &response.UserResponse{
		Id:             user.ID,
		Email:          user.Email,
		Name:           user.Name,
		OrganisationID: user.OrganisationID,
	}
	c.JSON(http.StatusOK, userDetails)
}

func NewUserController(logger *zap.Logger) *UserController {
	return &UserController{
		logger: logger.Named("UserController"),
	}
}
