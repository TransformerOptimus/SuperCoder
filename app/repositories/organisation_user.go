package repositories

import (
	"ai-developer/app/models"
	"fmt"
	"gorm.io/gorm"
)

type OrganisationUserRepository struct {
	db *gorm.DB
}

func (receiver OrganisationUserRepository) CreateOrganisationUser(tx *gorm.DB, organisationUser *models.OrganisationUser) (*models.OrganisationUser, error) {
	if tx == nil {
		return nil, fmt.Errorf("transaction is null")
	}
	err := tx.Create(organisationUser).Error
	if err != nil {
		return nil, err
	}
	return organisationUser, nil
}

func (receiver OrganisationUserRepository) GetOrganisationUserByUserID(userID uint) (*models.OrganisationUser, error) {
	var organisationUser *models.OrganisationUser
	err := receiver.db.First(&organisationUser, userID).Error
	if err != nil {
		return nil, err
	}
	return organisationUser, nil
}

func (receiver OrganisationUserRepository) GetOrganisationUserByOrganisationID(organisationID string) (*models.OrganisationUser, error) {
	var organisationUser *models.OrganisationUser
	err := receiver.db.First(&organisationUser, organisationID).Error
	if err != nil {
		return nil, err
	}
	return organisationUser, nil
}

func NewOrganisationUserRepository(db *gorm.DB) *OrganisationUserRepository {
	return &OrganisationUserRepository{
		db: db,
	}
}

func (receiver OrganisationUserRepository) GetDB() *gorm.DB {
	return receiver.db
}
