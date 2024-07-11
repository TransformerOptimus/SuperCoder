package controllers

import (
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type PullRequestController struct {
	pullRequestService     *services.PullRequestService
	userService            *services.UserService
	executionOutputService *services.ExecutionOutputService
	storyService           *services.StoryService
}

func NewPullRequestController(service *services.PullRequestService, userService *services.UserService,
	executionOutputService *services.ExecutionOutputService, storyService *services.StoryService) *PullRequestController {
	return &PullRequestController{
		pullRequestService:     service,
		userService:            userService,
		executionOutputService: executionOutputService,
	}
}

func (ctrl *PullRequestController) GetAllPullRequestsByProjectID(c *gin.Context) {
	projectIdStr := c.Param("project_id")
	projectID, err := strconv.Atoi(projectIdStr)
	status := c.Query("status")

	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pullRequests, err := ctrl.pullRequestService.GetAllPullRequests(projectID, status)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"pull_requests": pullRequests})
}

func (ctrl *PullRequestController) MergePullRequest(c *gin.Context) {
	var mergePullRequest request.MergePullRequest
	if err := c.ShouldBindJSON(&mergePullRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	email, _ := c.Get("email")
	user, err := ctrl.userService.GetUserByEmail(email.(string))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}
	organisationId := user.OrganisationID
	mergeSHA, err := ctrl.pullRequestService.MergePullRequestByID(mergePullRequest.PullRequestID, organisationId)
	if err != nil {
		fmt.Println("Error while merging Pull Request", err.Error())
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"merge_sha": mergeSHA})
}
func (ctrl *PullRequestController) FetchPullRequestCommits(c *gin.Context) {
	pullRequestIdStr := c.Param("pull_request_id")
	pullRequestID, err := strconv.Atoi(pullRequestIdStr)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	email, _ := c.Get("email")
	user, err := ctrl.userService.GetUserByEmail(email.(string))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}
	organisationId := user.OrganisationID
	allCommits, err := ctrl.pullRequestService.GetPullRequestsCommits(pullRequestID, int(organisationId))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"all_commits": allCommits})
}

func (ctrl *PullRequestController) GetPullRequestDiffByPullRequestID(c *gin.Context) {
	pullRequestIdStr := c.Param("pull_request_id")
	pullRequestID, err := strconv.Atoi(pullRequestIdStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid execution ID"})
		return
	}

	diff, err := ctrl.pullRequestService.GetPullRequestDiffByPullRequestID(uint(pullRequestID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"diff": diff})
}

func (ctrl *PullRequestController) CreatePullRequestFromCodeEditor(c *gin.Context) {
	var createPRRequest request.CreatePRFromCodeEditorRequest
	if err := c.ShouldBindJSON(&createPRRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ProjectID := createPRRequest.ProjectID
	Title := createPRRequest.Title
	Description := createPRRequest.Description
	fmt.Println("project id _____", ProjectID)
	prId, err := ctrl.pullRequestService.CreatePullRequestFromCodeEditor(ProjectID, Title, Description)
	if err !=nil{
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create Pull Request"})
        return
	}

	c.JSON(http.StatusOK, gin.H{"pull_request_id": prId})
}