package models

import (
	"time"
)

type ExecutionOutput struct {
	ID          uint      `gorm:"primaryKey"`
	ExecutionID uint      `gorm:"not null"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}
