package repositories

import (
	"ai-developer/app/models"
	"gorm.io/gorm"
)

type ActivityLogRepository struct {
	db *gorm.DB
}

func NewActivityLogRepository(db *gorm.DB) *ActivityLogRepository {
	return &ActivityLogRepository{db: db}
}

func (r *ActivityLogRepository) CreateActivityLog(executionID uint, executionStepID uint, logType string, logMessage string) error {
	activityLog := models.ActivityLog{
		ExecutionID:     executionID,
		ExecutionStepID: executionStepID,
		LogMessage:      logMessage,
		Type:            logType,
	}

	return r.db.Create(&activityLog).Error
}

func (r *ActivityLogRepository) GetActivityLogsByExecutionID(executionID uint) ([]models.ActivityLog, error) {
	var logs []models.ActivityLog
	if err := r.db.Where("execution_id = ?", executionID).Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *ActivityLogRepository) GetActivityLogsByExecutionIDs(executionIDs []uint) ([]models.ActivityLog, error) {
	var logs []models.ActivityLog
	if err := r.db.Where("execution_id IN ?", executionIDs).Order("created_at ASC").Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *ActivityLogRepository) CreateActivityLogWithTx(tx *gorm.DB, executionID uint, executionStepID uint, logType string, logMessage string) error {
	activityLog := models.ActivityLog{
		ExecutionID:     executionID,
		ExecutionStepID: executionStepID,
		LogMessage:      logMessage,
		Type:            logType,
	}

	return tx.Create(&activityLog).Error
}
