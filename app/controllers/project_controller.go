package controllers

import (
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strconv"
)

type ProjectController struct {
	projectService      *services.ProjectService
	codeDownloadService *services.CodeDownloadService
	userService         *services.UserService
	logger              *zap.Logger
}

func (controller *ProjectController) GetAllProjects(context *gin.Context) {
	email, _ := context.Get("email")
	user, err := controller.userService.GetUserByEmail(email.(string))
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}
	organisationId := user.OrganisationID
	allProjects, err := controller.projectService.GetAllProjectsOfOrganisation(int(organisationId))
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, allProjects)
	return
}

func (controller *ProjectController) GetProjectById(context *gin.Context) {
	projectIdStr := context.Param("project_id")
	projectId, err := strconv.Atoi(projectIdStr)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "project_id must be an integer"})
		return
	}

	project, err := controller.projectService.GetProjectDetailsById(projectId)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	context.JSON(http.StatusOK, project)
}

func (controller *ProjectController) CreateProject(context *gin.Context) {
	var createProjectRequest request.CreateProjectRequest
	if err := context.ShouldBindJSON(&createProjectRequest); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	email, _ := context.Get("email")
	user, err := controller.userService.GetUserByEmail(email.(string))
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}
	project, err := controller.projectService.CreateProject(int(user.OrganisationID), createProjectRequest)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, gin.H{"project_id": project.ID, "project_url": project.Url, "project_name": project.Name,
		"project_frontend_url": project.FrontendURL, "project_backend_url": project.BackendURL, "project_framework": project.Framework, "project_frontend_framework": project.FrontendFramework})
	return 
}

func (controller *ProjectController) UpdateProject(context *gin.Context) {
	var updateProjectRequest request.UpdateProjectRequest
	if err := context.ShouldBindJSON(&updateProjectRequest); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updatedProject, err := controller.projectService.UpdateProject(updateProjectRequest)
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	context.JSON(http.StatusOK, updatedProject)
	return
}

func (controller *ProjectController) DownloadCode(context *gin.Context) {
	projectId, err := strconv.ParseUint(context.Param("project_id"), 0, 64)
	if err != nil {
		return
	}
	zipFile, err := controller.codeDownloadService.GetZipFile(uint(projectId))
	if err != nil {
		context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer func(path string, logger *zap.Logger) {
		err = os.Remove(path)
		if err != nil {
			logger.Error("Error deleting zip file", zap.String("path", path), zap.Error(err))
		}
	}(*zipFile, controller.logger)
	context.Header("Content-Description", "File Transfer")
	context.Header("Content-Disposition", "attachment; filename=code.zip")
	context.Header("Content-Type", "application/zip")
	context.FileAttachment(*zipFile, "code.zip")
	return
}

func NewProjectController(
	projectService *services.ProjectService,
	userService *services.UserService,
	codeDownloadService *services.CodeDownloadService,
	logger *zap.Logger,
) *ProjectController {
	return &ProjectController{
		projectService:      projectService,
		userService:         userService,
		codeDownloadService: codeDownloadService,
		logger:              logger,
	}
}
