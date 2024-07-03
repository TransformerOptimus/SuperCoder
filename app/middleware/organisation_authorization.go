package middleware

import (
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
		userID, exists := c.Get("user_id")
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
		userIDInt, ok := userID.(int)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type"})
			return
		}
		// Fetch user details
		user, err := m.userService.GetUserByID(uint(userIDInt))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
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
