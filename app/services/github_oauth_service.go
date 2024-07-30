package services

import (
	"ai-developer/app/models"
	"context"
	"errors"
	"fmt"
	goGithub "github.com/google/go-github/github"
	"golang.org/x/oauth2"
	oauthGithub "golang.org/x/oauth2/github"
)

type GithubOauthService struct {
	jwtService          *JWTService
	userService         *UserService
	organisationService *OrganisationService
	clientID            string
	clientSecret        string
	redirectURL         string
}

func (s *GithubOauthService) HandleGithubCallback(code string) (string, string, string, string, int, error) {
	var githubOauthConfig = &oauth2.Config{
		ClientID:     s.clientID,
		ClientSecret: s.clientSecret,
		RedirectURL:  s.redirectURL,
		Scopes:       []string{"user:email"},
		Endpoint:     oauthGithub.Endpoint,
	}
	token, err := githubOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		return "", "", "", "", 0, err
	}

	client := goGithub.NewClient(githubOauthConfig.Client(context.Background(), token))
	githubUser, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		return "", "", "", "", 0, err
	}
	emails, _, err := client.Users.ListEmails(context.Background(), nil)
	if err != nil {
		return "", "", "", "", 0, err
	}

	// Extract the primary email address
	var primaryEmail string
	for _, email := range emails {
		if email.GetPrimary() {
			primaryEmail = email.GetEmail()
			break
		}
	}
	var name string
	if githubUser.Login != nil {
		name = *githubUser.Login
	} else {
		name = "N/A"
	}
	user, err := s.userService.GetUserByEmail(primaryEmail)
	var userExists string

	if err != nil {
		if user == nil {
			organisation := &models.Organisation{
				Name: s.organisationService.CreateOrganisationName(),
			}
			_, err = s.organisationService.CreateOrganisation(organisation)
			hashedPassword, err := s.userService.HashUserPassword(s.userService.CreatePassword())
			if err != nil {
				fmt.Println("Error while hashing password: ", err.Error())
				return "", "", "", "", 0, errors.New("error while creating user")
			}
			user = &models.User{
				Name:           name,
				Email:          primaryEmail,
				OrganisationID: organisation.ID,
				Password:       hashedPassword,
			}
			_, err = s.userService.CreateUser(user)
			if err != nil {
				return "", "", "", "", 0, err
			}
			userExists = "no"
		}
	}
	user.Name = name
	err = s.userService.UpdateUserByEmail(primaryEmail, user)
	if err != nil {
		return "", "", "", "", 0, err
	}
	userExists = "yes"
	accessToken, err := s.jwtService.GenerateToken(int(user.ID), primaryEmail)
	if err != nil {
		return "", "", "", "", 0, err
	}

	return accessToken, name, primaryEmail, userExists, int(user.OrganisationID), nil
}

func NewGithubOauthService(userService *UserService, jwtService *JWTService, organisationService *OrganisationService, clientID string, clientSecret string, redirectURL string) *GithubOauthService {
	return &GithubOauthService{
		userService:         userService,
		jwtService:          jwtService,
		organisationService: organisationService,
		clientID:            clientID,
		clientSecret:        clientSecret,
		redirectURL:         redirectURL,
	}
}
