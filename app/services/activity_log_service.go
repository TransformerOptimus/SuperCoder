package services

import (
	"ai-developer/app/models"
	"ai-developer/app/repositories"
	"gorm.io/gorm"
	"sort"
)

type ActivityLogService struct {
	ActivityLogRepo *repositories.ActivityLogRepository
	ExecutionRepo   *repositories.ExecutionRepository
}

func NewActivityLogService(activityLogRepo *repositories.ActivityLogRepository, executionRepo *repositories.ExecutionRepository) *ActivityLogService {
	return &ActivityLogService{
		ActivityLogRepo: activityLogRepo,
		ExecutionRepo:   executionRepo,
	}
}

func (s *ActivityLogService) GetActivityLogsByStoryID(storyID uint) (models.ActivityLogResponse, error) {
	executions, err := s.ExecutionRepo.GetExecutionsByStoryID(storyID)
	if err != nil {
		return models.ActivityLogResponse{}, err
	}

	if len(executions) == 0 {
		return models.ActivityLogResponse{}, nil
	}

	sort.Slice(executions, func(i, j int) bool {
		return executions[i].CreatedAt.After(executions[j].CreatedAt)
	})

	latestExecution := executions[0]
	status := latestExecution.Status

	var executionIDs []uint
	for _, execution := range executions {
		executionIDs = append(executionIDs, execution.ID)
	}

	logs, err := s.ActivityLogRepo.GetActivityLogsByExecutionIDs(executionIDs)
	if err != nil {
		return models.ActivityLogResponse{}, err
	}

	return models.ActivityLogResponse{Logs: logs, Status: status}, nil
}

func (s *ActivityLogService) CreateActivityLog(executionID uint, executionStepID uint, logType string, logMessage string) error {
	return s.ActivityLogRepo.CreateActivityLog(executionID, executionStepID, logType, logMessage)
}

func (s *ActivityLogService) GetActivityLogsByExecutionID(executionID uint) ([]models.ActivityLog, error) {
	return s.ActivityLogRepo.GetActivityLogsByExecutionID(executionID)
}

func (s *ActivityLogService) CreateActivityLogWithTx(tx *gorm.DB, executionID uint, executionStepID uint, logType string, logMessage string) error {
	return s.ActivityLogRepo.CreateActivityLogWithTx(tx, executionID, executionStepID, logType, logMessage)
}
