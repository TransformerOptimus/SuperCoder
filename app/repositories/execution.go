package repositories

import (
	"ai-developer/app/models"
	"fmt"
	"gorm.io/gorm"
	"time"
)

type ExecutionRepository struct {
	db *gorm.DB
}

func NewExecutionRepository(db *gorm.DB) *ExecutionRepository {
	return &ExecutionRepository{db: db}
}

// CreateExecution creates a new execution entry in the database.
func (r *ExecutionRepository) CreateExecution(storyID uint, instruction string, reExecute bool, branchName string) (*models.Execution, error) {
	execution := &models.Execution{
		StoryID:     storyID,
		BranchName:  branchName,
		Instruction: instruction,
		Status:      "IN_PROGRESS",
		ReExecution: reExecute,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := r.db.Create(execution).Error; err != nil {
		return nil, err
	}
	return execution, nil

}

// GetExecutionByID fetches an execution by its ID.
func (r *ExecutionRepository) GetExecutionByID(id uint) (*models.Execution, error) {
	fmt.Println("Fetching execution by ID: ", id)
	var execution models.Execution
	if err := r.db.First(&execution, id).Error; err != nil {
		return nil, err
	}
	return &execution, nil
}

// UpdateCommitID updates the commit ID of an execution.
func (r *ExecutionRepository) UpdateCommitID(execution *models.Execution, commitID string) error {
	execution.GitCommitID = commitID
	execution.UpdatedAt = time.Now()
	return r.db.Save(execution).Error
}

// GetExecutionsByStoryID fetches all executions by a story ID.
func (r *ExecutionRepository) GetExecutionsByStoryID(storyID uint) ([]models.Execution, error) {
	var executions []models.Execution
	if err := r.db.Where("story_id = ?", storyID).Find(&executions).Error; err != nil {
		return nil, err
	}
	return executions, nil
}

// UpdateStatus updates the status of an execution.
func (r *ExecutionRepository) UpdateStatus(executionID uint, newStatus string) error {
	var execution models.Execution
	if err := r.db.First(&execution, executionID).Error; err != nil {
		return err
	}
	execution.Status = newStatus
	execution.UpdatedAt = time.Now()
	return r.db.Save(&execution).Error
}

// Get Execution by story id and status
func (r *ExecutionRepository) GetExecutionByStoryIDAndStatus(storyID uint, status string) (*models.Execution, error) {
	var execution models.Execution
	if err := r.db.Where("story_id = ? AND status = ?", storyID, status).First(&execution).Error; err != nil {
		return nil, err
	}
	return &execution, nil
}

// CreateExecutionWithTx creates a new execution entry in the database within a transaction.
func (r *ExecutionRepository) CreateExecutionWithTx(tx *gorm.DB, storyID uint, instruction string, reExecute bool, branchName string) (*models.Execution, error) {
	execution := &models.Execution{
		StoryID:     storyID,
		BranchName:  branchName,
		Instruction: instruction,
		Status:      "IN_PROGRESS",
		ReExecution: reExecute,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := tx.Create(execution).Error; err != nil {
		return nil, err
	}
	return execution, nil
}

func (r *ExecutionRepository) GetExecutionsInProgress() ([]*models.Execution, error) {
	var executions []*models.Execution
	if err := r.db.Where("status = ?", "IN_PROGRESS").Find(&executions).Error; err != nil {
		return nil, err
	}
	return executions, nil
}

func (r *ExecutionRepository) GetExecutionsWithStatusAndCreatedAtRange(status string, startTime time.Time, endTime time.Time) ([]*models.Execution, error) {
	var executions []*models.Execution
	query := r.db.Where("status = ?", status)
	if !startTime.IsZero() {
		query = query.Where("created_at > ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("created_at < ?", endTime)
	}
	if err := query.Find(&executions).Error; err != nil {
		return nil, err
	}
	return executions, nil
}

func (r *ExecutionRepository) GetExecutionsByBranchName(branchName string) (*models.Execution, error) {
	var execution models.Execution
    if err := r.db.Where("branch_name = ?", branchName).Find(&execution).Error; err != nil {
        return nil, err
    }
    return &execution, nil
}

