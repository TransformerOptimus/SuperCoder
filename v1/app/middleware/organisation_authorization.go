package middleware

import (
	"ai-developer/app/models"
	"ai-developer/app/services"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type OrganizationAuthorizationMiddleware struct {
	userService         *services.UserService
	organisationService *services.OrganisationService
}

func NewOrganizationAuthorizationMiddleware(userService *services.UserService, organisationService *services.OrganisationService) *OrganizationAuthorizationMiddleware {
	return &OrganizationAuthorizationMiddleware{
		userService:         userService,
		organisationService: organisationService,
	}
}

func (m *OrganizationAuthorizationMiddleware) Authorize() gin.HandlerFunc {
	return func(c *gin.Context) {
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

		// Extract organization ID from the URL
		organisationIDStr := c.Param("organisation_id")
		organisationID, err := strconv.Atoi(organisationIDStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
			return
		}

		// Authorization logic: Check if the user belongs to the specified organization
		if user.OrganisationID != uint(organisationID) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}

		c.Next()
	}
}
