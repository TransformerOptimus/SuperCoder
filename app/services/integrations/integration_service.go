package integrations

import (
	"ai-developer/app/models"
	"ai-developer/app/models/types"
	"ai-developer/app/repositories"
	"go.uber.org/zap"
)

type IntegrationService struct {
	integrationsRepository *repositories.IntegrationsRepository
	logger                 *zap.Logger
}

func (is *IntegrationService) FindIntegrationIdByUserIdAndType(userId uint64, integrationType string) (integration *models.Integration, err error) {
	return is.integrationsRepository.FindIntegrationIdByUserIdAndType(userId, integrationType)
}

func (is *IntegrationService) DeleteIntegration(userId uint64, integrationType string) (err error) {
	return is.integrationsRepository.DeleteIntegration(userId, integrationType)
}

func (is *IntegrationService) AddOrUpdateIntegration(
	userId uint64,
	integrationType string,
	accessToken string,
	refreshToken *string,
	metadata *types.JSONMap,
) (err error) {
	return is.integrationsRepository.AddOrUpdateIntegration(
		userId,
		integrationType,
		accessToken,
		refreshToken,
		metadata,
	)
}

func NewIntegrationService(
	integrationsRepository *repositories.IntegrationsRepository,
	logger *zap.Logger,
) *IntegrationService {
	return &IntegrationService{
		integrationsRepository: integrationsRepository,
		logger:                 logger.Named("IntegrationService"),
	}
}
