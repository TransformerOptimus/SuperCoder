package models

import (
	"ai-developer/app/models/types"
	"time"
)

// ExecutionStep represents a step in the execution workflow.
type ExecutionStep struct {
	ID          uint          `gorm:"primaryKey"`
	ExecutionID uint          `gorm:"not null"`
	Name        string        `gorm:"type:varchar(100);not null"` // Name of the step (e.g. "GENERATE_CODE")
	Type        string        `gorm:"type:varchar(50);not null"`
	Request     types.JSONMap `gorm:"type:json"`
	Response    types.JSONMap `gorm:"type:json"`
	Status      string        `gorm:"type:varchar(50);not null"` // IN_PROGRESS, SUCCESS, FAILURE
	CreatedAt   time.Time     `gorm:"autoCreateTime"`
	UpdatedAt   time.Time     `gorm:"autoUpdateTime"`
}
