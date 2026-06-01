package repositories

import (
	"ai-developer/app/models"
	"gorm.io/gorm"
	"time"
)

type ExecutionOutputRepository struct {
	db *gorm.DB
}

func NewExecutionOutputRepository(db *gorm.DB) *ExecutionOutputRepository {
	return &ExecutionOutputRepository{db: db}
}

func (r *ExecutionOutputRepository) CreateExecutionOutput(executionID uint) (*models.ExecutionOutput, error) {
	executionOutput := &models.ExecutionOutput{
		ExecutionID: executionID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := r.db.Create(executionOutput).Error; err != nil {
		return nil, err
	}
	return executionOutput, nil
}

func (r *ExecutionOutputRepository) GetExecutionOutputByID(id uint) (*models.ExecutionOutput, error) {
	var executionOutput models.ExecutionOutput
	if err := r.db.First(&executionOutput, id).Error; err != nil {
		return nil, err
	}
	return &executionOutput, nil
}

func (r *ExecutionOutputRepository) GetExecutionOutputsByExecutionIDs(executionIDs []uint) ([]models.ExecutionOutput, error) {
	var executionOutputs []models.ExecutionOutput
	if err := r.db.Where("execution_id IN ?", executionIDs).Find(&executionOutputs).Error; err != nil {
		return nil, err
	}
	return executionOutputs, nil
}
