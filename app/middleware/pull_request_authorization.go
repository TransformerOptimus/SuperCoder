package middleware

import (
	"ai-developer/app/models"
	"ai-developer/app/services"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type PullRequestAuthorizationMiddleware struct {
	pullRequestService *services.PullRequestService
	userService        *services.UserService
}

func NewPullRequestAuthorizationMiddleware(pullRequestService *services.PullRequestService, userService *services.UserService) *PullRequestAuthorizationMiddleware {
	return &PullRequestAuthorizationMiddleware{
		pullRequestService: pullRequestService,
		userService:        userService,
	}
}

func (m *PullRequestAuthorizationMiddleware) Authorize() gin.HandlerFunc {
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

		pullRequestIDStr := c.Param("pull_request_id")
		pullRequestID, err := strconv.Atoi(pullRequestIDStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid pull request ID"})
			return
		}

		project, err := m.pullRequestService.GetPullRequestWithDetails(uint(pullRequestID))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pull request details"})
			return
		}

		if user.OrganisationID != project.OrganisationID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}

		c.Next()
	}
}
