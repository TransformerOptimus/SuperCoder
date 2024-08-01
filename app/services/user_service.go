package services

import (
	"ai-developer/app/models"
	"ai-developer/app/repositories"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"math/rand"
)

type UserService struct {
	userRepo   *repositories.UserRepository
	orgService *OrganisationService
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

func (s *UserService) HandleUserSignUp(email string, password string) (user *models.User, err error) {
	organisation := &models.Organisation{
		Name: s.orgService.CreateOrganisationName(),
	}

	organisation, err = s.orgService.CreateOrganisation(organisation)
	if err != nil {
		fmt.Println("Error while creating organization: ", err.Error())
		return nil, err
	}

	hashedPassword, err := s.HashUserPassword(password)
	if err != nil {
		fmt.Println("Error while hashing password: ", err.Error())
		return nil, err
	}

	user = &models.User{
		Name:           email,
		Email:          email,
		OrganisationID: organisation.ID,
		Password:       hashedPassword,
	}
	user, err = s.CreateUser(user)
	if err != nil {
		fmt.Println("Error while creating user: ", err.Error())
		return nil, err
	}
	return
}

func (s *UserService) HashUserPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

func (s *UserService) VerifyUserPassword(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
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

func NewUserService(userRepo *repositories.UserRepository, orgService *OrganisationService) *UserService {
	return &UserService{
		userRepo:   userRepo,
		orgService: orgService,
	}
}
