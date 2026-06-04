package models

import (
	"time"
)

type Execution struct {
	ID          uint      `gorm:"primaryKey"`
	StoryID     uint      `gorm:"not null"`
	Plan        string    `gorm:"type:text"`
	Status      string    `gorm:"type:varchar(100);not null"`
	BranchName  string    `gorm:"type:varchar(100);not null"`
	GitCommitID string    `gorm:"type:varchar(100)"`
	Instruction string    `gorm:"type:text;not null"`
	ReExecution bool      `gorm:"default:false"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}
