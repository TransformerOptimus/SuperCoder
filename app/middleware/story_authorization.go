package middleware

import (
	"ai-developer/app/models"
	"ai-developer/app/services"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type StoryAuthorizationMiddleware struct {
	storyService   *services.StoryService
	projectService *services.ProjectService
	userService    *services.UserService
}

func NewStoryAuthorizationMiddleware(storyService *services.StoryService, projectService *services.ProjectService, userService *services.UserService) *StoryAuthorizationMiddleware {
	return &StoryAuthorizationMiddleware{
		storyService:   storyService,
		projectService: projectService,
		userService:    userService,
	}
}

func (m *StoryAuthorizationMiddleware) Authorize() gin.HandlerFunc {
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

		storyIDStr := c.Param("story_id")
		storyID, err := strconv.Atoi(storyIDStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid story ID"})
			return
		}

		story, err := m.storyService.GetStoryById(int64(storyID))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch story"})
			return
		}

		project, err := m.projectService.GetProjectDetailsById(int(story.ProjectID))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch project"})
			return
		}

		if user.OrganisationID != project.OrganisationID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}

		c.Next()
	}
}
