package models

import (
	"time"
)

type StoryFile struct {
	ID        uint      `gorm:"primaryKey"`
	StoryID   uint      `gorm:"not null"`
	Name      string    `gorm:"type:varchar(100);not null"` // Name of the file (e.g. "input.txt") Migration to be added
	FilePath  string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}
