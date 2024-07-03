package repositories

import (
	"ai-developer/app/models"
	"fmt"
	"gorm.io/gorm"
	"time"
)

type ExecutionStepRepository struct {
	db *gorm.DB
}

func NewExecutionStepRepository(db *gorm.DB) *ExecutionStepRepository {
	return &ExecutionStepRepository{
		db: db,
	}
}

// CreateExecutionStep creates a new execution step entry in the database.
func (executionStepRepository *ExecutionStepRepository) CreateExecutionStep(executionID uint, name, stepType string, request map[string]interface{}) (*models.ExecutionStep, error) {
	fmt.Println("Creating Execution Step!")
	if request == nil {
		request = make(map[string]interface{})
	}
	executionStep := &models.ExecutionStep{
		ExecutionID: executionID,
		Name:        name,
		Type:        stepType,
		Request:     request,
		Status:      "IN_PROGRESS",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	fmt.Printf("Execution Step: %v\n", executionStep)
	if err := executionStepRepository.db.Create(executionStep).Error; err != nil {
		return nil, err
	}
	return executionStep, nil
}

// UpdateExecutionStepResponse updates the response and status of an execution step.
func (executionStepRepository *ExecutionStepRepository) UpdateExecutionStepResponse(executionStep *models.ExecutionStep, response map[string]interface{}, status string) error {
	executionStep.Response = response
	executionStep.Status = status
	executionStep.UpdatedAt = time.Now()
	return executionStepRepository.db.Save(executionStep).Error
}

func (executionStepRepository *ExecutionStepRepository) UpdateExecutionStepRequest(executionStep *models.ExecutionStep, request map[string]interface{}, status string) error {
	executionStep.Request = request
	executionStep.Status = status
	executionStep.UpdatedAt = time.Now()
	return executionStepRepository.db.Save(executionStep).Error
}

func (executionStepRepository *ExecutionStepRepository) UpdateExecutionStepStatus(executionStep *models.ExecutionStep, status string) error {
	executionStep.Status = status
	executionStep.UpdatedAt = time.Now()
	return executionStepRepository.db.Save(executionStep).Error
}

func (executionStepRepository *ExecutionStepRepository) FetchExecutionSteps(executionID uint, name, stepType string, limit int) ([]models.ExecutionStep, error) {
	var steps []models.ExecutionStep
	if err := executionStepRepository.db.Where("name = ? AND type = ? AND execution_id = ?", name, stepType, executionID).Order("created_at desc").Limit(limit).Find(&steps).Error; err != nil {
		return nil, err
	}
	return steps, nil
}

func (executionStepRepository *ExecutionStepRepository) CountExecutionStepsOfType(executionID uint, stepType string) (int64, error) {
	var count int64
	if err := executionStepRepository.db.Model(&models.ExecutionStep{}).
		Where("execution_id = ? AND type = ?", executionID, stepType).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (executionStepRepository *ExecutionStepRepository) CountExecutionStepsOfName(executionID uint, name string) (int64, error) {
	var count int64
	if err := executionStepRepository.db.Model(&models.ExecutionStep{}).
		Where("execution_id = ? AND name = ?", executionID, name).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil

}

func (executionStepRepository *ExecutionStepRepository) FetchExecutionStepByID(id uint) (*models.ExecutionStep, error) {
	var executionStep models.ExecutionStep
	if err := executionStepRepository.db.First(&executionStep, id).Error; err != nil {
		return nil, err
	}
	return &executionStep, nil
}

// CreateExecutionStepWithTx creates a new execution step entry in the database within a transaction.
func (executionStepRepository *ExecutionStepRepository) CreateExecutionStepWithTx(tx *gorm.DB, executionID uint, name, stepType string, request map[string]interface{}) (*models.ExecutionStep, error) {
	fmt.Println("Creating Execution Step within a transaction!")
	if request == nil {
		request = make(map[string]interface{})
	}
	executionStep := &models.ExecutionStep{
		ExecutionID: executionID,
		Name:        name,
		Type:        stepType,
		Request:     request,
		Status:      "IN_PROGRESS",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	fmt.Printf("Execution Step: %v\n", executionStep)
	if err := tx.Create(executionStep).Error; err != nil {
		return nil, err
	}
	return executionStep, nil
}
