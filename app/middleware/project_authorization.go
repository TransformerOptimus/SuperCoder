package middleware

import (
	"ai-developer/app/services"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type ProjectAuthorizationMiddleware struct {
	projectService *services.ProjectService
	userService    *services.UserService
}

func NewProjectAuthorizationMiddleware(projectService *services.ProjectService, userService *services.UserService) *ProjectAuthorizationMiddleware {
	return &ProjectAuthorizationMiddleware{
		projectService: projectService,
		userService:    userService,
	}
}

func (m *ProjectAuthorizationMiddleware) Authorize() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		projectIDStr := c.Param("project_id")
		projectID, err := strconv.Atoi(projectIDStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
			return
		}

		project, err := m.projectService.GetProjectDetailsById(projectID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch project"})
			return
		}

		userIDInt, ok := userID.(int)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type"})
			return
		}
		user, err := m.userService.GetUserByID(uint(userIDInt))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
			return
		}

		if user.OrganisationID != project.OrganisationID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}

		c.Next()
	}
}
