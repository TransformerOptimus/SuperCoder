package models

import (
	"time"
)

type OrganisationUser struct {
	ID             uint      `gorm:"primaryKey"`
	UserID         uint      `gorm:"type:varchar(100);not null"`
	OrganisationID uint      `gorm:"not null"`
	IsActive       bool      `gorm:"default:false"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}
