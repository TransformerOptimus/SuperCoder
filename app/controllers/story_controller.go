package controllers

import (
	"ai-developer/app/models/types"
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"mime/multipart"
	"net/http"
	"strconv"
)

type StoryController struct {
	storyService     *services.StoryService
	executionService *services.ExecutionService
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

func (controller *StoryController) CreateDesignStory(context *gin.Context) {
	file, err := context.FormFile("file")
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	if file == nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "File not found"})
		return
	}
	title := context.PostForm("title")
	uploadedFile, err := file.Open()
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer func(uploadedFile multipart.File) {
		err := uploadedFile.Close()
		if err != nil {
			context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}(uploadedFile)
	projectIdStr := context.PostForm("project_id")
	storyType := "frontend"
	projectId, err := strconv.Atoi(projectIdStr)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project_id"})
		return
	}
	storyID, err := controller.storyService.CreateDesignStoryForProject(uploadedFile, file.Filename, title, projectId, storyType)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"story_id": storyID})
}

func (controller *StoryController) GetAllStoriesOfProject(context *gin.Context) {
	projectIdStr := context.Param("project_id")
	searchValue := context.Query("search")
	storyType := context.Query("story_type")

	projectID, err := strconv.Atoi(projectIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	stories, err := controller.storyService.GetAllStoriesOfProject(projectID, searchValue, storyType)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	fmt.Println(stories)
	context.JSON(http.StatusOK, gin.H{"stories": stories})
}

func (controller *StoryController) GetDesignStoriesOfProject(context *gin.Context) {
	projectIdStr := context.Param("project_id")
	projectID, err := strconv.Atoi(projectIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	storyType := "frontend"
	stories, err := controller.storyService.GetDesignStoriesOfProject(projectID, storyType)
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
		return
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

func (controller *StoryController) EditDesignStoryById(context *gin.Context) {
	storyIdStr := context.PostForm("story_id")
	storyID, err := strconv.Atoi(storyIdStr)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "Invalid story_id"})
		return
	}
	title := context.PostForm("title")
	file, err := context.FormFile("file")
	if err != nil && err != http.ErrMissingFile {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if file == nil {
		err = controller.storyService.UpdateDesignStory(nil, "", title, storyID)
	} else {
		uploadedFile, err := file.Open()
		if err != nil {
			context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer func(uploadedFile multipart.File) {
			err := uploadedFile.Close()
			if err != nil {
				context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}(uploadedFile)
		err = controller.storyService.UpdateDesignStory(uploadedFile, file.Filename, title, storyID)
	}
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
		return
	}
	story, err := controller.storyService.GetInProgressStoriesByProjectId(projectID)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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

func (controller *StoryController) GetCodeForDesignStory(context *gin.Context) {
	storyIdStr := context.Param("story_id")
	storyID, err := strconv.Atoi(storyIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	story, err := controller.storyService.GetCodeForDesignStory(storyID)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"code_files": story})
}

func (controller *StoryController) GetDesignStoryByID(context *gin.Context) {
	storyIdStr := context.Param("story_id")
	storyID, err := strconv.Atoi(storyIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	story, err := controller.storyService.GetDesignStoryDetails(storyID)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"story": story})
}

func (controller *StoryController) UpdateStoryIsReviewed(context *gin.Context) {
	storyIdStr := context.Param("story_id")
	storyID, err := strconv.Atoi(storyIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	err = controller.storyService.UpdateReviewViewed(storyID, true)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"status": "OK"})
}

func NewStoryController(storyService *services.StoryService, executionService *services.ExecutionService) *StoryController {
	return &StoryController{
		storyService:     storyService,
		executionService: executionService,
	}
}
