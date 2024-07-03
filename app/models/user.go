package models

import (
	"time"
)

type User struct {
	ID             uint      `gorm:"primaryKey"`
	Name           string    `gorm:"type:varchar(100);not null"`
	Email          string    `gorm:"type:varchar(100);uniqueIndex;not null"`
	Password       string    `gorm:"type:varchar(100);not null"`
	OrganisationID uint      `gorm:"not null"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}
