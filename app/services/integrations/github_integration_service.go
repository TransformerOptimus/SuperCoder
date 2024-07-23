package integrations

import (
	"ai-developer/app/config"
	"ai-developer/app/models/dtos/integrations"
	"ai-developer/app/utils"
	"context"
	"errors"
	"fmt"
	"github.com/google/go-github/github"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	githubOAuth "golang.org/x/oauth2/github"
	"gorm.io/gorm"
	"strconv"
)

type GithubIntegrationService struct {
	logger *zap.Logger

	oauthConfig             oauth2.Config
	githubIntegrationConfig *config.GithubIntegrationConfig

	integrationService *IntegrationService
}

func (gis *GithubIntegrationService) DeleteIntegration(userId uint64) (err error) {
	err = gis.integrationService.DeleteIntegration(userId, GithubIntegrationType)
	return
}

func (gis *GithubIntegrationService) GetRedirectUrl(userId uint64) string {
	return gis.oauthConfig.AuthCodeURL(fmt.Sprintf("%d", userId), oauth2.AccessTypeOnline)
}

func (gis *GithubIntegrationService) HasGithubIntegration(userId uint64) (hasIntegration bool, err error) {
	integration, err := gis.integrationService.FindIntegrationIdByUserIdAndType(userId, GithubIntegrationType)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return
	}
	hasIntegration = integration != nil
	return
}

func (gis *GithubIntegrationService) GetRepositories(userId uint64) (repos []*github.Repository, err error) {
	integration, err := gis.integrationService.FindIntegrationIdByUserIdAndType(userId, GithubIntegrationType)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return make([]*github.Repository, 0), nil
	}

	if err != nil {
		return
	}

	client := github.NewClient(gis.oauthConfig.Client(context.Background(), &oauth2.Token{
		AccessToken: integration.AccessToken,
	}))

	repos, _, err = client.Repositories.List(context.Background(), "", &github.RepositoryListOptions{
		ListOptions: github.ListOptions{
			PerPage: 500,
		},
	})

	if err != nil {
		gis.logger.Error("Error getting github repositories", zap.Error(err))
		return
	}
	return
}

func (gis *GithubIntegrationService) GetGithubIntegrationDetails(userId uint64) (integrationsDetails *integrations.GithubIntegrationDetails, err error) {
	integration, err := gis.integrationService.FindIntegrationIdByUserIdAndType(userId, GithubIntegrationType)
	if err != nil || integration == nil {
		return
	}

	client := github.NewClient(gis.oauthConfig.Client(context.Background(), &oauth2.Token{
		AccessToken: integration.AccessToken,
	}))
	githubUser, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		gis.logger.Error("Error getting github user", zap.Error(err))
		return
	}

	integrationsDetails = &integrations.GithubIntegrationDetails{
		UserId:       integration.UserId,
		AccessToken:  integration.AccessToken,
		RefreshToken: integration.RefreshToken,
		GithubUserId: githubUser.GetLogin(),
	}

	return
}

func (gis *GithubIntegrationService) GenerateAndSaveAccessToken(code string, state string) (err error) {
	token, err := gis.oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		gis.logger.Error("Error exchanging code for token", zap.Error(err))
		return
	}

	userId, err := strconv.ParseUint(state, 10, 64)
	if err != nil {
		gis.logger.Error("Error parsing state to userId", zap.Error(err))
		return
	}

	if userId == 0 {
		gis.logger.Error("Invalid userId", zap.Uint64("userId", userId))
		return
	}

	client := github.NewClient(gis.oauthConfig.Client(context.Background(), token))
	githubUser, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		gis.logger.Error("Error getting github user", zap.Error(err))
		return
	}

	metadata, err := utils.GetAsJsonMap(githubUser)
	if err != nil {
		gis.logger.Error("Error getting github user as json map", zap.Error(err))
		return
	}

	gis.logger.Info(
		"Adding or updating github integration",
		zap.Uint64("userId", userId),
		zap.Any("metadata", metadata),
	)

	var refreshToken *string
	if token.RefreshToken != "" {
		refreshToken = &token.RefreshToken
	}

	err = gis.integrationService.AddOrUpdateIntegration(
		userId,
		GithubIntegrationType,
		token.AccessToken,
		refreshToken,
		metadata,
	)

	return
}

func NewGithubIntegrationService(
	logger *zap.Logger,
	githubIntegrationConfig *config.GithubIntegrationConfig,
	integrationService *IntegrationService,
) *GithubIntegrationService {

	oauthConfig := oauth2.Config{
		RedirectURL:  githubIntegrationConfig.GetRedirectURL(),
		ClientID:     githubIntegrationConfig.GetClientID(),
		ClientSecret: githubIntegrationConfig.GetClientSecret(),
		Scopes:       []string{"user:email", "repo"},
		Endpoint:     githubOAuth.Endpoint,
	}

	return &GithubIntegrationService{
		logger:                  logger.Named("GithubIntegrationService"),
		oauthConfig:             oauthConfig,
		integrationService:      integrationService,
		githubIntegrationConfig: githubIntegrationConfig,
	}
}
