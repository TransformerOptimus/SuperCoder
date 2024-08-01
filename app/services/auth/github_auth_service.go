package auth

import (
	"ai-developer/app/config"
	"ai-developer/app/models"
	"ai-developer/app/services"
	"context"
	"errors"
	"github.com/google/go-github/github"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	oauthGithub "golang.org/x/oauth2/github"
	"gorm.io/gorm"
)

type GithubAuthService struct {
	logger              *zap.Logger
	userService         *services.UserService
	organisationService *services.OrganisationService
	githubOAuthConfig   *config.GithubOAuthConfig
}

func (gas GithubAuthService) GetRedirectUrl(state string) string {
	var githubOauthConfig = &oauth2.Config{
		RedirectURL:  gas.githubOAuthConfig.RedirectURL(),
		ClientID:     gas.githubOAuthConfig.ClientId(),
		ClientSecret: gas.githubOAuthConfig.ClientSecret(),
		Scopes:       []string{"user:email"},
		Endpoint:     oauthGithub.Endpoint,
	}
	return githubOauthConfig.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (gas GithubAuthService) HandleGithubCallback(code string, state string) (*models.User, error) {
	githubOauthConfig := &oauth2.Config{
		ClientID:     gas.githubOAuthConfig.ClientId(),
		ClientSecret: gas.githubOAuthConfig.ClientSecret(),
		RedirectURL:  gas.githubOAuthConfig.RedirectURL(),
		Scopes:       []string{"user:email"},
		Endpoint:     oauthGithub.Endpoint,
	}

	token, err := githubOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		gas.logger.Error("Error exchanging code for token", zap.Error(err))
		return nil, nil
	}

	client := github.NewClient(githubOauthConfig.Client(context.Background(), token))

	emails, _, err := client.Users.ListEmails(context.Background(), nil)
	if err != nil {
		gas.logger.Error("Error fetching user emails", zap.Error(err))
		return nil, nil
	}

	var primaryEmail string
	for _, email := range emails {
		if email.GetPrimary() {
			primaryEmail = email.GetEmail()
			break
		}
	}

	existingUser, err := gas.userService.GetUserByEmail(primaryEmail)

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		gas.logger.Error("Error fetching user by email", zap.Error(err))
		return nil, nil
	}

	if existingUser != nil {
		gas.logger.Debug("User authenticated with Github", zap.Any("user", existingUser))
		return existingUser, nil
	}

	gas.logger.Debug("User not found, creating new user")
	err = nil
	var githubUser *github.User
	githubUser, _, err = client.Users.Get(context.Background(), "")
	if err != nil {
		gas.logger.Error("Error fetching user from Github", zap.Error(err))
		return nil, nil
	}

	return gas.CreateUser(primaryEmail, githubUser)
}

func (gas GithubAuthService) CreateUser(email string, githubUser *github.User) (user *models.User, err error) {
	var name string
	if githubUser.Login != nil {
		name = *githubUser.Login
	} else {
		name = "N/A"
	}

	organisation := &models.Organisation{
		Name: gas.organisationService.CreateOrganisationName(),
	}
	_, err = gas.organisationService.CreateOrganisation(organisation)
	if err != nil {
		gas.logger.Error("Error creating organisation", zap.Error(err))
		return
	}

	hashedPassword, err := gas.userService.HashUserPassword(gas.userService.CreatePassword())
	if err != nil {
		gas.logger.Error("Error hashing user password", zap.Error(err))
		return
	}

	user, err = gas.userService.CreateUser(&models.User{
		Name:           name,
		Email:          email,
		OrganisationID: organisation.ID,
		Password:       hashedPassword,
	})

	if err != nil {
		gas.logger.Error("Error creating user", zap.Error(err))
		return
	}

	return
}

func NewGithubAuthService(
	githubOAuthConfig *config.GithubOAuthConfig,
	userService *services.UserService,
	organisationService *services.OrganisationService,
	logger *zap.Logger,
) *GithubAuthService {
	return &GithubAuthService{
		logger:              logger,
		userService:         userService,
		organisationService: organisationService,
		githubOAuthConfig:   githubOAuthConfig,
	}
}
