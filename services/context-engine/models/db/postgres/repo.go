package postgres

import (
	"time"

	"gorm.io/gorm"
)

type Repo struct {
	ID          uint           `gorm:"primaryKey;autoIncrement"`
	UserID      string         `gorm:"type:varchar(255);not null;uniqueIndex:idx_repo_identity"`
	WorkspaceID uint64         `gorm:"not null;uniqueIndex:idx_repo_identity"`
	MachineID   string         `gorm:"type:varchar(255);not null;uniqueIndex:idx_repo_identity"`
	RepoPath    string         `gorm:"type:text;not null;uniqueIndex:idx_repo_identity"`
	RepoURL     string         `gorm:"type:text"`
	CreatedAt   time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt   time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (Repo) TableName() string { return "repos" }
