package models

import (
	"time"
)

type Story struct {
	ID           uint      `gorm:"primaryKey"`
	ProjectID    uint      `gorm:"not null"`
	Title        string    `gorm:"type:varchar(100);not null"`
	Description  string    `gorm:"type:text"`
	Status       string    `gorm:"type:varchar(50)"`
	IsDeleted    bool      `gorm:"default:false"`
	HashID       string    `gorm:"type:varchar(100);unique"`
	Url          string    `gorm:"type:varchar(100)"`
	FrontendURL  string    `gorm:"type:varchar(255)"`
	ReviewViewed bool      `gorm:"default:false"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
	Type         string    `gorm:"type;not null"`
}
