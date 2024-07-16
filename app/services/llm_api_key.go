package services

import (
	"ai-developer/app/constants"
	"ai-developer/app/models"
	"ai-developer/app/models/dtos/llm_api_key"
	"ai-developer/app/repositories"
	"errors"
	"gorm.io/gorm"
)

type LLMAPIKeyService struct {
	llm_api_key_repo *repositories.LLMAPIKeyRepository
}

func (s *LLMAPIKeyService) CreateOrUpdateLLMAPIKey(organisationID uint, llmModel string, llmAPIKey string) error {
	if llmModel == "" || llmAPIKey == "" {
		return errors.New("missing required fields")
	}
	err := s.llm_api_key_repo.CreateOrUpdateLLMAPIKey(organisationID, llmModel, llmAPIKey)
	if err != nil {
		return err
	}
	return nil
}

func (s *LLMAPIKeyService) GetLLMAPIKeyByModelName(llmmodel string, organisationID uint) (*models.LLMAPIKey, error) {
	return s.llm_api_key_repo.GetLLMAPIKeyByModelNameAndOrganisationID(llmmodel, organisationID)
}

func (s *LLMAPIKeyService) GetAllLLMAPIKeyByOrganisationID(organisationID uint) ([]llm_api_key.LLMAPIKeyReturn, error) {
	var llmAPIKeys []llm_api_key.LLMAPIKeyReturn

	llmModelList := []string{constants.GPT_4O, constants.CLAUDE_3}
	for _, llmModel := range llmModelList {
		apiKey, err := s.GetLLMAPIKeyByModelName(llmModel, organisationID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				llmAPIKeys = append(llmAPIKeys, llm_api_key.LLMAPIKeyReturn{ModelName: llmModel, APIKey: ""})
				continue
			}
			return nil, err
		}
		llmAPIKeys = append(llmAPIKeys, llm_api_key.LLMAPIKeyReturn{ModelName: apiKey.LLMModel, APIKey: apiKey.LLMAPIKey})
	}
	return llmAPIKeys, nil
}

func NewLLMAPIKeyService(llm_api_key_repo *repositories.LLMAPIKeyRepository) *LLMAPIKeyService {
	return &LLMAPIKeyService{
		llm_api_key_repo: llm_api_key_repo,
	}
}
