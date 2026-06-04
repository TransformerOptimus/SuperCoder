package auth

import (
	"ai-developer/app/config"
	"ai-developer/app/models"
	"ai-developer/app/services"
	"errors"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type EmailAuthService struct {
	logger              *zap.Logger
	userService         *services.UserService
	organisationService *services.OrganisationService
	githubOAuthConfig   *config.GithubOAuthConfig
}

func (gas EmailAuthService) HandleSignIn(email string, password string) (user *models.User, err error) {
	existingUser, err := gas.userService.GetUserByEmail(email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("unable to get user details")
	}

	if existingUser == nil {
		return nil, errors.New("user not found")
	}

	validated := gas.userService.VerifyUserPassword(password, existingUser.Password)
	if !validated {
		return nil, errors.New("invalid password")
	}

	user = existingUser
	return
}

func (gas EmailAuthService) HandleSignUp(email string, password string) (user *models.User, err error) {
	user, err = gas.userService.GetUserByEmail(email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("unable to check user details")
	}

	if user == nil {
		user, err = gas.userService.HandleUserSignUp(email, password)
		return
	} else {
		return nil, errors.New("user already exists")
	}
}

func NewEmailAuthService(
	githubOAuthConfig *config.GithubOAuthConfig,
	userService *services.UserService,
	organisationService *services.OrganisationService,
	logger *zap.Logger,
) *EmailAuthService {
	return &EmailAuthService{
		logger:              logger,
		userService:         userService,
		organisationService: organisationService,
		githubOAuthConfig:   githubOAuthConfig,
	}
}
