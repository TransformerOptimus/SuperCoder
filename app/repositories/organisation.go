package repositories

import (
	"ai-developer/app/models"
	"gorm.io/gorm"
)

type OrganisationRepository struct {
	db *gorm.DB
}

func (receiver OrganisationRepository) CreateOrganisation(organisation *models.Organisation) (*models.Organisation, error) {
	err := receiver.db.Create(organisation).Error
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
