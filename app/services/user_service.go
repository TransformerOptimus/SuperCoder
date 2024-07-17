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

func (s *UserService) HandleUserSignUp(request request.CreateUserRequest, inviteToken string) (*models.User, string, error) {
	var err error
	var inviteOrganisationId int
	newUser := &models.User{
		Name:     request.Email,
		Email:    request.Email,
		Password: request.Password,
	}
	if inviteToken != "" {
		_, inviteOrganisationId, err = s.jwtService.DecodeInviteToken(inviteToken)
		if err != nil {
			return nil, "", err
		}
	}
	newUser, err = s.handleNewUserOrg(newUser, inviteOrganisationId)
	newUser, err = s.CreateUser(newUser)
	if err != nil {
		fmt.Println("Error while creating user: ", err.Error())
		return nil, "", err
	}
	_, err = s.createOrganisationUser(newUser)
	var accessToken, jwtErr = s.jwtService.GenerateToken(int(newUser.ID), newUser.Email)
	if jwtErr != nil {
		fmt.Println(" Jwt error: ", accessToken, jwtErr.Error())
		return nil, "", nil
	}
	return newUser, accessToken, nil
}

func (s *UserService) handleNewUserOrg(user *models.User, inviteOrgId int) (*models.User, error) {
	if inviteOrgId == 0 {
		organisation := &models.Organisation{
			Name: s.orgService.CreateOrganisationName(),
		}
		_, err := s.orgService.CreateOrganisation(organisation)
		if err != nil {
			return nil, err
		}
		user.OrganisationID = organisation.ID
	} else {
		user.OrganisationID = uint(inviteOrgId)
	}
	return user, nil
}

func (s *UserService) createOrganisationUser(user *models.User) (*models.OrganisationUser, error) {
	return s.organisationUserRepo.CreateOrganisationUser(s.organisationUserRepo.GetDB(), &models.OrganisationUser{
		OrganisationID: user.OrganisationID,
		UserID:         user.ID,
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})
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
