package repositories

import (
	"ai-developer/app/models"
	"ai-developer/app/models/types"
	"errors"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type IntegrationsRepository struct {
	db     *gorm.DB
	logger *zap.Logger
}

func (ir *IntegrationsRepository) FindIntegrationIdByUserIdAndType(userId uint64, integrationType string) (integration *models.Integration, err error) {
	err = ir.db.Model(models.Integration{
		UserId:          userId,
		IntegrationType: integrationType,
	}).First(&integration).Error
	if err != nil {
		return nil, err
	}
	return
}

func (ir *IntegrationsRepository) DeleteIntegration(userId uint64, integrationType string) (err error) {
	ir.logger.Info(
		"Deleting integration",
		zap.Uint64("userId", userId),
		zap.String("integrationType", integrationType),
	)
	err = ir.db.Unscoped().Where(&models.Integration{
		UserId:          userId,
		IntegrationType: integrationType,
	}).Delete(&models.Integration{
		UserId:          userId,
		IntegrationType: integrationType,
	}).Error
	return
}

func (ir *IntegrationsRepository) AddOrUpdateIntegration(
	userId uint64,
	integrationType string,
	accessToken string,
	refreshToken *string,
	metadata *types.JSONMap,
) (err error) {
	integration, err := ir.FindIntegrationIdByUserIdAndType(userId, integrationType)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if integration != nil {
		ir.logger.Info(
			"Updating integration",
			zap.Uint64("userId", integration.UserId),
			zap.String("integrationType", integration.IntegrationType),
		)
		integration.AccessToken = accessToken
		integration.RefreshToken = refreshToken
		integration.Metadata = metadata
		return ir.db.Save(integration).Error
	} else {
		integration = &models.Integration{
			UserId:          userId,
			IntegrationType: integrationType,
			AccessToken:     accessToken,
			RefreshToken:    refreshToken,
			Metadata:        metadata,
		}
		ir.logger.Info(
			"Adding new integration",
			zap.Uint64("userId", userId),
			zap.String("integrationType", integrationType),
		)
		return ir.db.Create(integration).Error
	}
}

func NewIntegrationsRepository(db *gorm.DB, logger *zap.Logger) *IntegrationsRepository {
	return &IntegrationsRepository{
		db:     db,
		logger: logger.Named("IntegrationsRepository"),
	}
}
