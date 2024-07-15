package models

import (
	"time"
)

type DesignStoryReview struct {
	ID        uint      `gorm:"primaryKey"`
	StoryID   uint      `gorm:"not null"`
	Comment   string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}
