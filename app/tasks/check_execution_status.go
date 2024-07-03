package tasks

import (
	"ai-developer/app/constants"
	"ai-developer/app/monitoring"
	"ai-developer/app/services"
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

type CheckExecutionStatusTaskHandler struct {
	executionService *services.ExecutionService
	alertService     *monitoring.SlackAlert
	logger           *zap.Logger
}

func NewCheckExecutionStatusTaskHandler(
	executionService *services.ExecutionService,
	alertService *monitoring.SlackAlert,
	logger *zap.Logger) *CheckExecutionStatusTaskHandler {
	return &CheckExecutionStatusTaskHandler{
		executionService: executionService,
		alertService:     alertService,
		logger:           logger,
	}
}

func (h *CheckExecutionStatusTaskHandler) HandleTask(ctx context.Context, t *asynq.Task) error {
	h.logger.Info("Running CheckExecutionStatusTaskHandler.........")
	executions, err := h.executionService.GetExecutionsWithStatusAndCreatedAtRange(constants.InProgress, time.Now().Add(-1*time.Hour), time.Now().Add(-30*time.Minute))
	if err != nil {
		h.logger.Error("Failed to get in-progress executions", zap.Error(err))
		return fmt.Errorf("get in-progress executions: %w", err)
	}

	for _, execution := range executions {
		err = h.alertService.SendAlert("Execution has been in progress for more than 30 minutes",
			map[string]string{
				"story_id":     fmt.Sprintf("%d", execution.StoryID),
				"execution_id": fmt.Sprintf("%d", execution.ID),
			})
		if err != nil {
			h.logger.Error("Failed to send execution alert", zap.Error(err))
			return fmt.Errorf("send execution alert: %w", err)
		}
		h.logger.Info("Sent alert for long-running execution", zap.Uint("execution_id", execution.ID))
	}

	return nil
}
