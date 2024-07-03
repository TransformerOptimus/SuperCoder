package services

import (
	"ai-developer/app/models"
	"ai-developer/app/repositories"
	"fmt"
)

type ExecutionOutputService struct {
	executionOutputRepo *repositories.ExecutionOutputRepository
	executionRepository *repositories.ExecutionRepository
}

func NewExecutionOutputService(executionOutputRepo *repositories.ExecutionOutputRepository,
	executionRepository *repositories.ExecutionRepository,
) *ExecutionOutputService {
	return &ExecutionOutputService{
		executionRepository: executionRepository,
		executionOutputRepo: executionOutputRepo,
	}
}

func (s *ExecutionOutputService) GetExecutionOutputsByStoryID(storyID uint) ([]models.ExecutionOutput, error) {
	executions, err := s.executionRepository.GetExecutionsByStoryID(storyID)
	if err != nil {
		return nil, err
	}
	var executionIDs []uint
	for _, execution := range executions {
		executionIDs = append(executionIDs, execution.ID)
	}
	return s.executionOutputRepo.GetExecutionOutputsByExecutionIDs(executionIDs)
}

func (s *ExecutionOutputService) CreateExecutionOutput(executionId uint) (*models.ExecutionOutput, error) {
	executionOutput, err := s.executionOutputRepo.CreateExecutionOutput(executionId)
	if err != nil {
		fmt.Println("Error creating execution output: ", err)
		return nil, err
	}
	return executionOutput, nil
}

func (s *ExecutionOutputService) GetExecutionOutputByID(id uint) (*models.ExecutionOutput, error) {
	executionOutput, err := s.executionOutputRepo.GetExecutionOutputByID(id)
	if err != nil {
		fmt.Println("Error getting execution output: ", err)
		return nil, err
	}
	return executionOutput, nil
}
