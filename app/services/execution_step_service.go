package services

import (
	"ai-developer/app/models"
	"ai-developer/app/repositories"
	"gorm.io/gorm"
)

type ExecutionStepService struct {
	executionStepRepository *repositories.ExecutionStepRepository
}

func NewExecutionStepService(repo *repositories.ExecutionStepRepository) *ExecutionStepService {
	return &ExecutionStepService{
		executionStepRepository: repo,
	}
}

func (s *ExecutionStepService) CreateExecutionStep(executionID uint, name, stepType string, request map[string]interface{}) (*models.ExecutionStep, error) {
	return s.executionStepRepository.CreateExecutionStep(executionID, name, stepType, request)
}

func (s *ExecutionStepService) UpdateExecutionStepResponse(executionStep *models.ExecutionStep, response map[string]interface{}, status string) error {
	return s.executionStepRepository.UpdateExecutionStepResponse(executionStep, response, status)
}

func (s *ExecutionStepService) UpdateExecutionStepRequest(executionStep *models.ExecutionStep, request map[string]interface{}, status string) error {
	return s.executionStepRepository.UpdateExecutionStepRequest(executionStep, request, status)
}

func (s *ExecutionStepService) UpdateExecutionStepStatus(executionStep *models.ExecutionStep, status string) error {
	return s.executionStepRepository.UpdateExecutionStepStatus(executionStep, status)
}

func (s *ExecutionStepService) FetchExecutionSteps(executionID uint, name, stepType string, limit int) ([]models.ExecutionStep, error) {
	return s.executionStepRepository.FetchExecutionSteps(executionID, name, stepType, limit)
}

func (s *ExecutionStepService) CountExecutionStepsOfType(executionID uint, stepType string) (int64, error) {
	return s.executionStepRepository.CountExecutionStepsOfType(executionID, stepType)
}

func (s *ExecutionStepService) CountExecutionStepsOfName(executionID uint, name string) (int64, error) {
	return s.executionStepRepository.CountExecutionStepsOfName(executionID, name)
}

func (s *ExecutionStepService) FetchExecutionStepByID(id uint) (*models.ExecutionStep, error) {
	return s.executionStepRepository.FetchExecutionStepByID(id)
}

func (s *ExecutionStepService) CreateExecutionStepWithTx(tx *gorm.DB, executionID uint, name, stepType string, request map[string]interface{}) (*models.ExecutionStep, error) {
	return s.executionStepRepository.CreateExecutionStepWithTx(tx, executionID, name, stepType, request)
}
