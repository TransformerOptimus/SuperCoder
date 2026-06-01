package models

import (
	"time"
)

type StoryInstruction struct {
	ID          uint      `gorm:"primaryKey"`
	StoryID     uint      `gorm:"not null"`
	Instruction string    `gorm:"type:text;not null"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}
