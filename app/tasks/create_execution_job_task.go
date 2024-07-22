package tasks

import (
	"ai-developer/app/constants"
	"ai-developer/app/models/dtos/asynq_task"
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"ai-developer/app/utils"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/goccy/go-json"
	"gorm.io/gorm"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"ai-developer/app/client/workspace"
)

type CreateExecutionJobTaskHandler struct {
	workspaceServiceClient *workspace.WorkspaceServiceClient
	activityLogService     *services.ActivityLogService
	storyService           *services.StoryService
	executionService       *services.ExecutionService
	projectService         *services.ProjectService
	executionStepService   *services.ExecutionStepService
	pullRequestService     *services.PullRequestService
	executionOutputService *services.ExecutionOutputService
	db                     *gorm.DB
	logger                 *zap.Logger
}

func NewCreateExecutionJobTaskHandler(
	workspaceServiceClient *workspace.WorkspaceServiceClient,
	activityLogService *services.ActivityLogService,
	storyService *services.StoryService,
	executionService *services.ExecutionService,
	projectService *services.ProjectService,
	executionStepService *services.ExecutionStepService,
	pullRequestService *services.PullRequestService,
	executionOutputService *services.ExecutionOutputService,
	db *gorm.DB,
	logger *zap.Logger,
) *CreateExecutionJobTaskHandler {
	return &CreateExecutionJobTaskHandler{
		workspaceServiceClient: workspaceServiceClient,
		activityLogService:     activityLogService,
		storyService:           storyService,
		executionService:       executionService,
		projectService:         projectService,
		executionStepService:   executionStepService,
		pullRequestService:     pullRequestService,
		executionOutputService: executionOutputService,
		db:                     db,
		logger:                 logger,
	}
}

func (h *CreateExecutionJobTaskHandler) HandleTask(ctx context.Context, t *asynq.Task) error {
	var payload asynq_task.CreateJobPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v", err)
	}
	h.logger.Info("Handling CreateExecutionJobTask...", zap.Any("payload", payload))
	tx := h.db.Begin()
	if tx.Error != nil {
		h.logger.Error("Transaction failed", zap.Error(tx.Error))
		return tx.Error
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			h.logger.Error("Transaction failed and rolled back due to panic", zap.Any("error", r))
		}
	}()

	story, err := h.storyService.GetStoryById(int64(payload.StoryID))
	if err != nil {
		tx.Rollback()
		h.logger.Error("Error fetching story", zap.Error(err))
		return err
	}
	runningExecution, err := h.executionService.GetExecutionByStoryIdAndStatus(story.ID, constants.InProgress)

	if runningExecution != nil {
		tx.Rollback()
		h.logger.Error("Execution already in progress for this story")
		return errors.New("execution already in progress for this story")
	}

	project, err := h.projectService.GetProjectById(story.ProjectID)
	if err != nil {
		tx.Rollback()
		h.logger.Error("Error fetching project", zap.Error(err))
		return err
	}

	existingStoryInProgress, err := h.storyService.GetStoryByProjectIdAndStatus(int(project.ID), constants.InProgress)
	if existingStoryInProgress != nil {
		tx.Rollback()
		h.logger.Error("Story with status IN_PROGRESS already exists for this project")
		return errors.New("story with status IN_PROGRESS already exists for this project")
	}

	branchPrefix, err := utils.RandString(5)
	if err != nil {
		tx.Rollback()
		h.logger.Error("Error generating random string", zap.Error(err))
		return err
	}
	branchName := fmt.Sprintf("branch_%s_%d", branchPrefix, story.ID)

	if payload.ReExecute && story.Type != constants.Frontend {
		pullRequest, _ := h.pullRequestService.GetPullRequestByID(payload.PullRequestId)
		executionOutput, _ := h.executionOutputService.GetExecutionOutputByID(pullRequest.ExecutionOutputID)
		existingPullRequestExecution, _ := h.executionService.GetExecutionByID(executionOutput.ExecutionID)
		branchName = existingPullRequestExecution.BranchName
	}
	h.logger.Info("Branch name generated", zap.String("branchName", branchName))
	h.logger.Info("Updating story status to IN_PROGRESS", zap.Any("story", story))
	err = h.storyService.UpdateStoryStatusWithTx(tx, int(story.ID), constants.InProgress)
	if err != nil {
		tx.Rollback()
		h.logger.Error("Error updating story status", zap.Error(err))
		return err
	}

	execution, err := h.executionService.CreateExecutionWithTx(tx, story.ID, "", payload.ReExecute, branchName)
	if err != nil {
		tx.Rollback()
		h.logger.Error("Error creating execution", zap.Error(err))
		return err
	}

	executionStep, err := h.executionStepService.CreateExecutionStepWithTx(tx, execution.ID, "INITIALIZE_WORKSPACE", "LOG", nil)
	if err != nil {
		tx.Rollback()
		h.logger.Error("Error creating execution step", zap.Error(err))
		return err
	}

	createJobRequest := request.NewCreateJobRequest()
	createJobRequest.WithBranch(branchName)
	createJobRequest.WithStoryId(int64(payload.StoryID))
	createJobRequest.WithIsReExecution(payload.ReExecute)
	createJobRequest.WithExecutionId(int64(execution.ID))
	if story.Type == constants.Frontend {
		fmt.Println("Project Framework", project.FrontendFramework)
		mountPath := "/workspaces/stories/" + project.HashID + "/" + story.HashID
		createJobRequest.WithWorkspaceMountPath(mountPath)
		createJobRequest.WithProjectId(project.HashID)
		createJobRequest.WithExecutorImage("node")
		createJobRequest.Env = append(createJobRequest.Env, "EXECUTION_TEMPLATE="+strings.ToUpper(project.FrontendFramework))
	} else {
		fmt.Println("Project Framework", project.BackendFramework)
		createJobRequest.WithPullRequestId(int64(payload.PullRequestId))
		createJobRequest.WithProjectId(project.HashID)
		createJobRequest.WithExecutorImage("python")
		mountPath := "/workspaces/" + project.HashID
		createJobRequest.WithWorkspaceMountPath(mountPath)
		createJobRequest.Env = append(createJobRequest.Env, "EXECUTION_TEMPLATE="+strings.ToUpper(project.BackendFramework))

	}

	h.logger.Info("Payload for create job request", zap.Any("createJobRequest", createJobRequest))

	job, err := h.workspaceServiceClient.CreateJob(createJobRequest)
	if err != nil {
		tx.Rollback()
		h.logger.Error("Error creating job", zap.Error(err))
		return err
	}

	if payload.ReExecute {
		err = h.activityLogService.CreateActivityLogWithTx(tx, execution.ID, executionStep.ID, "INFO", fmt.Sprintf("Rebuilding %s story...", story.Type))
		if err != nil {
			tx.Rollback()
			h.logger.Error("Error creating activity log", zap.Error(err))
			return err
		}
	}
	err = h.activityLogService.CreateActivityLogWithTx(tx, execution.ID, executionStep.ID, "INFO",fmt.Sprintf("Initializing Workspace for automated %s development...", story.Type))
	if err != nil {
		tx.Rollback()
		h.logger.Error("Error creating activity log", zap.Error(err))
		return err
	}

	if err := tx.Commit().Error; err != nil {
		h.logger.Error("Transaction commit failed", zap.Error(err))
		return err
	}

	h.logger.Info("Job created and transaction committed", zap.Any("job", job))
	return nil
}
