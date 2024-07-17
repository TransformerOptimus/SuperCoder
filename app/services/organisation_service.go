package services

import (
	"ai-developer/app/config"
	"ai-developer/app/models"
	"ai-developer/app/repositories"
	"ai-developer/app/services/email"
	"ai-developer/app/services/git_providers"
	"ai-developer/app/types/request"
	"ai-developer/app/types/response"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type OrganisationService struct {
	organisationRepo *repositories.OrganisationRepository
	gitnessService   *git_providers.GitnessService
	userRepository   *repositories.UserRepository
	JWTService       *JWTService
	userRepo         *repositories.UserRepository
	emailService     email.EmailService
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
	tx := s.organisationRepo.GetDB().Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	org, err := s.organisationRepo.CreateOrganisation(tx, organisation)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	projectSpace, err := s.gitnessService.CreateProject(s.gitnessService.GetSpaceOrProjectName(org), s.gitnessService.GetSpaceOrProjectDescription(org))
	fmt.Println("Project/Space created: ", projectSpace)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return org, nil
}

func (s *OrganisationService) GetOrganisationByID(organisationID uint) (*models.Organisation, error) {
	return s.organisationRepo.GetOrganisationByID(organisationID)
}

func NewOrganisationService(organisationRepo *repositories.OrganisationRepository, gitnessService *git_providers.GitnessService, userRepository *repositories.UserRepository,
	emailService email.EmailService, jwtService *JWTService, userRepo *repositories.UserRepository) *OrganisationService {
	return &OrganisationService{
		organisationRepo: organisationRepo,
		gitnessService:   gitnessService,
		userRepository:   userRepository,
		emailService:     emailService,
		JWTService:       jwtService,
		userRepo:         userRepo,
	}
}

func (s *OrganisationService) GetOrganisationByName(organisationName string) (*models.Organisation, error) {
	return s.organisationRepo.GetOrganisationByName(organisationName)
}

func (s *OrganisationService) GetOrganizationUsers(organizationID uint) ([]*response.UserResponse, error) {
	var users, err = s.userRepository.FetchAllUsersByOrganizationId(organizationID)
	if err != nil {
		return nil, err
	}
	var usersResponse []*response.UserResponse

	for _, user := range users {
		mappedUser := &response.UserResponse{
			ID:             user.ID,
			Name:           user.Name,
			Email:          user.Email,
			OrganisationID: user.OrganisationID,
		}
		usersResponse = append(usersResponse, mappedUser)
	}

	return usersResponse, nil
}

func (s *OrganisationService) InviteUserToOrganization(organisationID int, userEmail string, currentUserID int) (*response.SendEmailResponse, error) {
	accessToken, err := s.JWTService.GenerateTokenForInvite(organisationID, userEmail)
	if err != nil {
		return &response.SendEmailResponse{
			Success:   false,
			MessageId: "",
			Error:     err.Error(),
		}, err
	}
	url := config.AppBackendUrl() + "organisation/handle_invite?invite_token=" + accessToken
	htmlContent, err := readFile(filepath.Join("app", "utils", "email_templates", "invite_email.html"))
	if err != nil {
		return &response.SendEmailResponse{
			Success:   false,
			MessageId: "",
			Error:     err.Error(),
		}, err
	}
	currentUser, err := s.userRepo.GetUserByID(uint(currentUserID))
	if err != nil {
		return &response.SendEmailResponse{
			Success:   false,
			MessageId: "",
			Error:     err.Error(),
		}, err
	}
	htmlContent = strings.Replace(htmlContent, "{invitor_email}", currentUser.Email, 1)
	htmlContent = strings.Replace(htmlContent, "{{ invite_url }}", url, 2)
	sendEmailRequest := &request.SendEmailRequest{
		ToEmail:     userEmail,
		Content:     url,
		HtmlContent: htmlContent,
		Subject:     "SuperCoder Invite",
	}
	return s.emailService.SendOutboundEmail(sendEmailRequest)
}

func readFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {

		}
	}(file)

	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	return string(content), nil
}
