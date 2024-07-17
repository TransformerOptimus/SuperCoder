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

func (receiver OrganisationUserRepository) GetOrganisationUserByUserIDAndOrganisationID(userID uint, organisationID uint) (*models.OrganisationUser, error) {
	var organisationUser models.OrganisationUser
	err := receiver.db.Where("user_id = ? AND organisation_id = ?", userID, organisationID).First(&organisationUser).Error
	if err != nil {
		return nil, err
	}
	return &organisationUser, nil
}

func NewOrganisationUserRepository(db *gorm.DB) *OrganisationUserRepository {
	return &OrganisationUserRepository{
		db: db,
	}
}

func (receiver OrganisationUserRepository) GetDB() *gorm.DB {
	return receiver.db
}
