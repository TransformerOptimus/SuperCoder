package services

import (
	"ai-developer/app/client/workspace"
	"ai-developer/app/models"
	"ai-developer/app/repositories"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"time"
)

type ExecutionService struct {
	ExecutionRepo          *repositories.ExecutionRepository
	ExecutionOutputRepo    *repositories.ExecutionOutputRepository
	PullRequestRepo        *repositories.PullRequestRepository
	activityLogService     *ActivityLogService
	workspaceServiceClient *workspace.WorkspaceServiceClient
	executionStepService   *ExecutionStepService
	asynqClient            *asynq.Client
	logger                 *zap.Logger
}

func NewExecutionService(
	executionRepo *repositories.ExecutionRepository,
	executionOutputRepo *repositories.ExecutionOutputRepository,
	PullRequestRepo *repositories.PullRequestRepository,
	workspaceServiceClient *workspace.WorkspaceServiceClient,
	activityLogService *ActivityLogService,
	executionStepService *ExecutionStepService,
	asynqClient *asynq.Client,
	logger *zap.Logger,
) *ExecutionService {
	return &ExecutionService{
		ExecutionRepo:          executionRepo,
		ExecutionOutputRepo:    executionOutputRepo,
		PullRequestRepo:        PullRequestRepo,
		workspaceServiceClient: workspaceServiceClient,
		activityLogService:     activityLogService,
		executionStepService:   executionStepService,
		asynqClient:            asynqClient,
		logger:                 logger,
	}
}

func (s *ExecutionService) CreateExecution(storyID uint, instruction string, reExecute bool, branchName string) (*models.Execution, error) {
	execution, err := s.ExecutionRepo.CreateExecution(storyID, instruction, reExecute, branchName)
	if err != nil {
		return nil, err
	}
	return execution, nil
}

func (s *ExecutionService) GetExecutionByID(executionID uint) (*models.Execution, error) {
	execution, err := s.ExecutionRepo.GetExecutionByID(executionID)
	if err != nil {
		return nil, err
	}
	return execution, nil
}

func (s *ExecutionService) UpdateExecutionStatus(executionID uint, newStatus string) error {
	return s.ExecutionRepo.UpdateStatus(executionID, newStatus)
}

func (s *ExecutionService) UpdateCommitID(execution *models.Execution, commitID string) error {
	return s.ExecutionRepo.UpdateCommitID(execution, commitID)
}

func (s *ExecutionService) GetExecutionByStoryIdAndStatus(storyID uint, status string) (*models.Execution, error) {
	execution, err := s.ExecutionRepo.GetExecutionByStoryIDAndStatus(storyID, status)
	if err != nil {
		return nil, err
	}
	return execution, nil
}

func (s *ExecutionService) CreateExecutionWithTx(tx *gorm.DB, storyID uint, instruction string, reExecute bool, branchName string) (*models.Execution, error) {
	execution, err := s.ExecutionRepo.CreateExecutionWithTx(tx, storyID, instruction, reExecute, branchName)
	if err != nil {
		return nil, err
	}
	return execution, nil
}

func (s *ExecutionService) GetExecutionsInProgress() ([]*models.Execution, error) {
	return s.ExecutionRepo.GetExecutionsInProgress()
}

func (s *ExecutionService) GetExecutionsWithStatusAndCreatedAtRange(status string, startTime time.Time, endTime time.Time) ([]*models.Execution, error) {
	return s.ExecutionRepo.GetExecutionsWithStatusAndCreatedAtRange(status, startTime, endTime)
}
