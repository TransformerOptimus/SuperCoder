package tasks

import (
	"ai-developer/app/services"
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"ai-developer/app/client/workspace"
	"ai-developer/app/models/dtos/asynq_task"
)

type DeleteWorkspaceTaskHandler struct {
	workspaceServiceClient *workspace.WorkspaceServiceClient
	projectService         *services.ProjectService
	logger                 *zap.Logger
}

func NewDeleteWorkspaceTaskHandler(
	workspaceServiceClient *workspace.WorkspaceServiceClient,
	projectService *services.ProjectService,
	logger *zap.Logger) *DeleteWorkspaceTaskHandler {
	return &DeleteWorkspaceTaskHandler{
		workspaceServiceClient: workspaceServiceClient,
		projectService:         projectService,
		logger:                 logger,
	}
}

func (h *DeleteWorkspaceTaskHandler) HandleTask(ctx context.Context, t *asynq.Task) error {
	var p asynq_task.CreateDeleteWorkspaceTaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		h.logger.Error("Failed to unmarshal payload", zap.Error(err))
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	h.logger.Info("Processing delete workspace task", zap.String("workspace_id", p.WorkspaceID))

	// Check the current active count
	activeCount, err := h.projectService.GetActiveProjectCount(p.WorkspaceID)
	if err != nil {
		h.logger.Error("Failed to get active project count", zap.Error(err))
		return fmt.Errorf("get active project count: %w", err)
	}

	if activeCount > 0 {
		h.logger.Info("Active count is greater than 0, skipping workspace deletion", zap.String("workspace_id", p.WorkspaceID))
		return nil
	}

	// Call the DeleteWorkspace API
	err = h.workspaceServiceClient.DeleteWorkspace(p.WorkspaceID)
	if err != nil {
		h.logger.Error("Failed to delete workspace", zap.Error(err))
		return fmt.Errorf("delete workspace: %w", err)
	}
	h.logger.Info("Successfully deleted workspace", zap.String("workspace_id", p.WorkspaceID))
	return nil
}
