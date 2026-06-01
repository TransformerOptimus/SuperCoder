package models

import (
	"time"
)

type PullRequest struct {
	ID                     uint      `gorm:"primaryKey"`
	StoryID                uint      `gorm:"not null"`
	ExecutionOutputID      uint      `gorm:"null"`
	PullRequestTitle       string    `gorm:"type:varchar(255);not null"`
	PullRequestNumber      int       `gorm:"not null"`
	Status                 string    `gorm:"type:varchar(255);not null"`
	PullRequestDescription string    `gorm:"type:text;not null"`
	PullRequestID          string    `gorm:"type:varchar(100);not null"`
	SourceSHA              string    `gorm:"type:varchar(100)"`
	MergeTargetSHA         string    `gorm:"type:varchar(100)"`
	MergeBaseSHA           string    `gorm:"type:varchar(100)"`
	RemoteType             string    `gorm:"type:varchar(50);not null"`
	CreatedAt              time.Time `gorm:"autoCreateTime"`
	UpdatedAt              time.Time `gorm:"autoUpdateTime"`
	MergedAt               time.Time `gorm:"autoUpdateTime"`
	ClosedAt               time.Time `gorm:"autoUpdateTime"`
	PRType                 string     `gorm:"type:varchar(50);not null"`
}
