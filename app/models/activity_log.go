package models

import (
	"time"
)

type ActivityLog struct {
	ID              uint      `gorm:"primaryKey"`
	ExecutionID     uint      `gorm:"not null"`
	ExecutionStepID uint      `gorm:"not null"`
	LogMessage      string    `gorm:"type:text;not null"`
	Type            string    `gorm:"type:varchar(50);not null"` // INFO, ERROR, WARNING, DEBUG
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

type ActivityLogResponse struct {
	Logs   []ActivityLog
	Status string
}
