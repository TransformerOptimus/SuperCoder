package repositories

import (
	"ai-developer/app/models"
	"errors"
	"gorm.io/gorm"
)

type LLMAPIKeyRepository struct {
	db *gorm.DB
}

func (receiver LLMAPIKeyRepository) CreateOrUpdateLLMAPIKey(organisationID uint, llmModel string, llmAPIKey string) error {
	existingAPIKey, err := receiver.GetLLMAPIKeyByModelNameAndOrganisationID(llmModel, organisationID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		newAPIKey := &models.LLMAPIKey{
			OrganisationID: organisationID,
			LLMModel:       llmModel,
			LLMAPIKey:      llmAPIKey,
		}
		if err := receiver.db.Create(newAPIKey).Error; err != nil {
			return err
		}
	} else {
		existingAPIKey.LLMAPIKey = llmAPIKey
		if err := receiver.db.Save(existingAPIKey).Error; err != nil {
			return err
		}
	}
	return nil
}

func (receiver LLMAPIKeyRepository) GetLLMAPIKeyByModelNameAndOrganisationID(modelName string, organisationID uint) (*models.LLMAPIKey, error) {
	var llmAPIKey models.LLMAPIKey
	err := receiver.db.Where("llm_model = ? AND organisation_id = ?", modelName, organisationID).First(&llmAPIKey).Error
	if err != nil {
		return nil, err
	}
	return &llmAPIKey, nil
}

func NewLLMAPIKeyRepository(db *gorm.DB) *LLMAPIKeyRepository {
	return &LLMAPIKeyRepository{
		db: db,
	}
}
