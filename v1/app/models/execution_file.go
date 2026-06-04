package models

import (
	"time"
)

type ExecutionFile struct {
	ID              uint      `gorm:"primaryKey"`
	ExecutionID     uint      `gorm:"not null"`
	ExecutionStepID uint      `gorm:"not null"`
	FilePath        string    `gorm:"type:text;not null"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}
