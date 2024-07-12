package models

import (
	"time"
)

type Project struct {
	ID                uint      `gorm:"primaryKey"`
	HashID            string    `gorm:"type:varchar(100);not null;unique"`
	Url               string    `gorm:"type:varchar(100)"`
	FrontendURL       string    `gorm:"type:varchar(100);"`
	BackendURL        string    `gorm:"type:varchar(100);"`
	Name              string    `gorm:"type:varchar(100);"`
	Framework         string    `gorm:"type:varchar(100);not null"`
	FrontendFramework string    `gorm:"type:varchar(100);not null"`
	Description       string    `gorm:"type:text"`
	OrganisationID    uint      `gorm:"not null"`
	CreatedAt         time.Time `gorm:"autoCreateTime"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime"`
}
