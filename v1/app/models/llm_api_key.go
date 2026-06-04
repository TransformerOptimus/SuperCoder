package models

type LLMAPIKey struct {
	ID             uint   `gorm:"primaryKey"`
	OrganisationID uint   `gorm:"not null"`
	LLMModel       string `gorm:"type:varchar(100)"`
	LLMAPIKey      string `gorm:"type:varchar(255);not null"`
}
