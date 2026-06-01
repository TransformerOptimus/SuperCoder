package models

import (
	"time"
)

type PullRequestComments struct {
	ID            uint      `gorm:"primaryKey"`
	StoryID       uint      `gorm:"not null"`
	PullRequestID uint      `gorm:"not null"`
	Comment       string    `gorm:"type:varchar(255);not null"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
}
