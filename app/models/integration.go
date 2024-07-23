package models

import (
	"ai-developer/app/models/types"
	"gorm.io/gorm"
)

type Integration struct {
	*gorm.Model
	ID uint `gorm:"primaryKey, autoIncrement"`

	UserId uint64 `gorm:"column:user_id;not null"`
	User   User   `gorm:"foreignKey:UserId;uniqueIndex:idx_user_integration"`

	IntegrationType string `gorm:"column:integration_type;type:varchar(255);not null;uniqueIndex:idx_user_integration"`

	AccessToken  string  `gorm:"type:varchar(255);not null"`
	RefreshToken *string `gorm:"type:varchar(255);null"`

	Metadata *types.JSONMap `gorm:"type:json;null"`
}
