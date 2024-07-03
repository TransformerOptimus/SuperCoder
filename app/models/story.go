package models

import (
	"time"
)

type Story struct {
	ID          uint      `gorm:"primaryKey"`
	ProjectID   uint      `gorm:"not null"`
	Title       string    `gorm:"type:varchar(100);not null"`
	Description string    `gorm:"type:text"`
	Status      string    `gorm:"type:varchar(50)"`
	IsDeleted   bool      `gorm:"default:false"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}
