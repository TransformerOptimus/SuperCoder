package repositories

import (
	"ai-developer/app/models"
	"fmt"
	"gorm.io/gorm"
)

type OrganisationRepository struct {
	db *gorm.DB
}

func (receiver OrganisationRepository) CreateOrganisation(tx *gorm.DB, organisation *models.Organisation) (*models.Organisation, error) {
	if tx == nil {
		return nil, fmt.Errorf("transaction is null")
	}
	err := tx.Create(organisation).Error
	if err != nil {
		return nil, err
	}
	return organisation, nil
}

func (receiver OrganisationRepository) GetOrganisationByID(organisationID uint) (*models.Organisation, error) {
	var organisation *models.Organisation
	err := receiver.db.First(&organisation, organisationID).Error
	if err != nil {
		return nil, err
	}
	return organisation, nil
}

func (receiver OrganisationRepository) GetOrganisationByName(organisationName string) (*models.Organisation, error) {
	var organisation *models.Organisation
	err := receiver.db.Where("name = ?", organisationName).First(&organisation).Error
	if err != nil {
		return nil, err
	}
	return organisation, nil
}

func NewOrganisationRepository(db *gorm.DB) *OrganisationRepository {
	return &OrganisationRepository{
		db: db,
	}
}

func (receiver OrganisationRepository) GetDB() *gorm.DB {
	return receiver.db
}
