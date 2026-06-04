package postgres

import (
	"time"

	"gorm.io/gorm"
)

type ShardAssignment struct {
	ID             uint           `gorm:"primaryKey;autoIncrement"`
	RepoID         uint           `gorm:"not null;uniqueIndex:idx_shard_repo_id"`
	Repo           Repo           `gorm:"foreignKey:RepoID;constraint:OnDelete:CASCADE"`
	CollectionName string         `gorm:"type:varchar(255);not null"`
	CreatedAt      time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt      time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (ShardAssignment) TableName() string { return "shard_assignments" }
