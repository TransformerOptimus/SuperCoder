package services

import (
	"ai-developer/app/config"
	"fmt"
	"net/url"
	"strconv"
)

type AuthService struct {
	userService *UserService
	jwtService  *JWTService
}

func NewAuthService(userService *UserService, jwtService *JWTService) *AuthService {
	return &AuthService{
		userService: userService,
		jwtService:  jwtService,
	}
}

func (s *AuthService) HandleDefaultAuth() (string, error) {
	defaultUser, err := s.userService.GetDefaultUser()
	if err != nil {
		return "", err
	}
	jwtToken, err := s.jwtService.GenerateToken(int(defaultUser.ID), defaultUser.Email)
	if err != nil {
		return "", err
	}
	redirectURL := fmt.Sprintf(config.GithubFrontendURL()+"/redirect?token=%s&name=%s&email=%s&user_exists=%s&organisation_id=%s",
		url.QueryEscape(jwtToken),
		url.QueryEscape(defaultUser.Name),
		url.QueryEscape(defaultUser.Email),
		url.QueryEscape("yes"),
		url.QueryEscape(strconv.Itoa(int(defaultUser.OrganisationID))),
	)
	return redirectURL, nil
}
