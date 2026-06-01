package models

import (
	"time"
)

type StoryTestCase struct {
	ID        uint      `gorm:"primaryKey"`
	StoryID   uint      `gorm:"not null"`
	TestCase  string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}
