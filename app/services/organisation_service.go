package services

import (
	"ai-developer/app/models"
	"ai-developer/app/repositories"
	"ai-developer/app/services/git_providers"
	"fmt"
	"math/rand"
	"time"
)

type OrganisationService struct {
	organisationRepo *repositories.OrganisationRepository
	gitnessService   *git_providers.GitnessService
}

func (s *OrganisationService) CreateOrganisationName() string {
	src := rand.NewSource(time.Now().UnixNano())
	r := rand.New(src)

	// Generate a random number between 0 and 999 (inclusive) using the new rand.Rand instance
	randomNumber := r.Intn(1000)

	// Format the number to be exactly 3 digits (e.g., 7 becomes "007")
	formattedNumber := fmt.Sprintf("%03d", randomNumber)

	// Create the organization name
	organizationName := "Organisation_" + formattedNumber

	return organizationName
}

func (s *OrganisationService) CreateOrganisation(organisation *models.Organisation) (*models.Organisation, error) {
	org, err := s.organisationRepo.CreateOrganisation(organisation)
	if err != nil {
		return nil, err

	}
	projectSpace, err := s.gitnessService.CreateProject(s.gitnessService.GetSpaceOrProjectName(org), s.gitnessService.GetSpaceOrProjectDescription(org))
	fmt.Println("Project/Space created: ", projectSpace)
	if err != nil {
		return nil, err
	}
	return org, nil
}

func (s *OrganisationService) GetOrganisationByID(organisationID uint) (*models.Organisation, error) {
	return s.organisationRepo.GetOrganisationByID(organisationID)
}

func (s *OrganisationService) GetOrganisationByName(organisationName string) (*models.Organisation, error) {
	return s.organisationRepo.GetOrganisationByName(organisationName)
}

func NewOrganisationService(organisationRepo *repositories.OrganisationRepository, gitnessService *git_providers.GitnessService) *OrganisationService {
	return &OrganisationService{
		organisationRepo: organisationRepo,
		gitnessService:   gitnessService,
	}
}
