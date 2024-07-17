package services

import (
	"ai-developer/app/models"
	"ai-developer/app/repositories"
	"ai-developer/app/types/request"
	"fmt"
	"math/rand"
	"time"
)

type UserService struct {
	userRepo             *repositories.UserRepository
	organisationUserRepo *repositories.OrganisationUserRepository
	orgService           *OrganisationService
	jwtService           *JWTService
}

func (s *UserService) GetUserByID(userID uint) (*models.User, error) {
	return s.userRepo.GetUserByID(userID)
}

func (s *UserService) GetUserByEmail(email string) (*models.User, error) {
	return s.userRepo.GetUserByEmail(email)
}

func (s *UserService) CreateUser(user *models.User) (*models.User, error) {
	return s.userRepo.CreateUser(user)
}

func (s *UserService) CreatePassword() string {
	length := 8
	b := make([]byte, length)
	for i := range b {
		switch rand.Intn(3) {
		case 0:
			b[i] = byte(rand.Intn(10)) + '0' // digits
		case 1:
			b[i] = byte(rand.Intn(26)) + 'A' // uppercase letters
		case 2:
			b[i] = byte(rand.Intn(26)) + 'a' // lowercase letters
		}
	}
	return string(b)
}

func (s *UserService) UpdateUserByEmail(email string, user *models.User) error {
	return s.userRepo.UpdateUserByEmail(email, user)
}

func (s *UserService) HandleUserSignUp(request request.CreateUserRequest) (*models.User, string, error) {
	newUser := &models.User{
		Name:     request.Email,
		Email:    request.Email,
		Password: request.Password,
	}
	if request.OrganisationID == nil {
		organisation := &models.Organisation{
			Name: s.orgService.CreateOrganisationName(),
		}
		var err error = nil
		organisation, err = s.orgService.CreateOrganisation(organisation)
		if err != nil {
			fmt.Println("Error while creating organization: ", err.Error())
			return nil, "", err
		}
		newUser.OrganisationID = organisation.ID
	} else {
		newUser.OrganisationID = *request.OrganisationID
	}
	newUser, err := s.CreateUser(newUser)
	if err != nil {
		fmt.Println("Error while creating user: ", err.Error())
		return nil, "", err
	}
	_, err = s.organisationUserRepo.CreateOrganisationUser(s.organisationUserRepo.GetDB(), &models.OrganisationUser{
		OrganisationID: newUser.OrganisationID,
		UserID:         newUser.ID,
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})
	var accessToken, jwtErr = s.jwtService.GenerateToken(int(newUser.ID), newUser.Email)
	if jwtErr != nil {
		fmt.Println(" Jwt error: ", accessToken, jwtErr.Error())
		return nil, "", nil
	}
	return newUser, accessToken, nil
}

func NewUserService(userRepo *repositories.UserRepository, orgService *OrganisationService, jwtService *JWTService,
	organisationUserRepo *repositories.OrganisationUserRepository) *UserService {
	return &UserService{
		userRepo:             userRepo,
		orgService:           orgService,
		jwtService:           jwtService,
		organisationUserRepo: organisationUserRepo,
	}
}

func (s *UserService) FetchOrganisationIDByUserID(userID uint) (uint, error) {
	return s.userRepo.FetchOrganisationIDByUserID(userID)
}

func (s *UserService) GetDefaultUser() (*models.User, error) {
	defaultUser, err := s.GetUserByEmail("supercoder@superagi.com")
	if err != nil {
		return nil, err
	}
	return defaultUser, nil
}
