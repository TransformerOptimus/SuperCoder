package controllers

import (
	"ai-developer/app/models/types"
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type StoryController struct {
	storyService *services.StoryService
}

func (controller *StoryController) CreateStory(context *gin.Context) {
	var createStoryRequest request.CreateStoryRequest
	if err := context.ShouldBindJSON(&createStoryRequest); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	storyID, err := controller.storyService.CreateStoryForProject(createStoryRequest)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"story_id": storyID})
}

func (controller *StoryController) GetAllStoriesOfProject(context *gin.Context) {
	projectIdStr := context.Param("project_id")
	searchValue := context.Query("search")

	projectID, err := strconv.Atoi(projectIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	stories, err := controller.storyService.GetAllStoriesOfProject(projectID, searchValue)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"stories": stories})
}

func (controller *StoryController) GetStoryById(context *gin.Context) {
	storyIdStr := context.Param("story_id")
	storyID, err := strconv.Atoi(storyIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	story, err := controller.storyService.GetStoryDetails(storyID)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	context.JSON(http.StatusOK, gin.H{"story": story})
}

func (controller *StoryController) UpdateStoryStatus(context *gin.Context) {
	storyIdStr := context.Param("story_id")
	storyID, err := strconv.Atoi(storyIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var updateStatusRequest request.UpdateStoryStatusRequest
	if err := context.ShouldBindJSON(&updateStatusRequest); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err = controller.storyService.UpdateStoryStatusByUser(storyID, updateStatusRequest.StoryStatus)
	if errors.Is(err, types.ErrInvalidStatus) ||
		errors.Is(err, types.ErrStoryDeleted) ||
		errors.Is(err, types.ErrInvalidStory) ||
		errors.Is(err, types.ErrInvalidStoryStatusTransition) {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"status": "OK"})
}

func (controller *StoryController) EditStoryByID(context *gin.Context) {
	storyIdStr := context.Param("story_id")
	_, err := strconv.Atoi(storyIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var updateStoryRequest request.UpdateStoryRequest
	if err := context.ShouldBindJSON(&updateStoryRequest); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = controller.storyService.UpdateStoryForProject(updateStoryRequest)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"status": "OK"})

}

func (controller *StoryController) GetInProgressStoriesByProjectId(context *gin.Context) {
	projectIdStr := context.Param("project_id")
	projectID, err := strconv.Atoi(projectIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	story, err := controller.storyService.GetInProgressStoriesByProjectId(projectID)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	context.JSON(http.StatusOK, gin.H{"story": story})
}

func (controller *StoryController) DeleteStoryById(context *gin.Context) {
	storyIdStr := context.Param("story_id")
	storyID, err := strconv.Atoi(storyIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err = controller.storyService.DeleteStoryByID(storyID)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"status": "OK"})
}

func NewStoryController(
	storyService *services.StoryService,
) *StoryController {
	return &StoryController{
		storyService: storyService,
	}
}
