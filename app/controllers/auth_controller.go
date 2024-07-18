package controllers

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"ai-developer/app/types/response"
	"fmt"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	oauthGithub "golang.org/x/oauth2/github"
	"net/http"
	"net/url"
	"strconv"
)

type AuthController struct {
	authService        *services.AuthService
	githubOauthService *services.GithubOauthService
	jwtService         *services.JWTService
	userService        *services.UserService
	clientID           string
	clientSecret       string
	redirectURL        string
}

func (controller *AuthController) GithubSignIn(c *gin.Context) {
	var env = config.Get("app.env")
	fmt.Println("ENV : ", env)
	if env == "development" {
		fmt.Println("Handling Skip Authentication Token.....")
		redirectURL, err := controller.authService.HandleDefaultAuth()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to get default user token"})
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, redirectURL)

	}
	var githubOauthConfig = &oauth2.Config{
		RedirectURL:  controller.redirectURL,
		ClientID:     controller.clientID,
		ClientSecret: controller.clientSecret,
		Scopes:       []string{"user:email"},
		Endpoint:     oauthGithub.Endpoint,
	}
	callback := githubOauthConfig.AuthCodeURL("state", oauth2.AccessTypeOnline)
	c.Redirect(http.StatusTemporaryRedirect, callback)
}

func (controller *AuthController) GithubCallback(c *gin.Context) {
	code := c.Query("code")
	accessToken, name, email, newExists, organisationId, err := controller.githubOauthService.HandleGithubCallback(code)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, config.GithubFrontendURL()+"/redirect?error="+err.Error())
		return
	}

	// Include the name and email in the redirect URL
	redirectURL := fmt.Sprintf(config.GithubFrontendURL()+"/redirect?token=%s&name=%s&email=%s&user_exists=%s&organisation_id=%s",
		url.QueryEscape(accessToken),
		url.QueryEscape(name),
		url.QueryEscape(email),
		url.QueryEscape(newExists),
		url.QueryEscape(strconv.Itoa(organisationId)),
	)

	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

func (controller *AuthController) SignUp(c *gin.Context) {
	var createUserRequest request.CreateUserRequest
	fmt.Println("Creating new user", createUserRequest.Email)
	if err := c.ShouldBindJSON(&createUserRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var existingUser, _ = controller.userService.GetUserByEmail(createUserRequest.Email)
	if existingUser == nil {
		var user, accessToken, err = controller.userService.HandleUserSignUp(createUserRequest)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "existing_user": false, "user": nil, "access_token": nil, "error": err.Error()})
			fmt.Println("Error occurred while creating new user : ", createUserRequest.Email, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "existing_user": false, "user": &response.UserResponse{
			Id:             user.ID,
			Name:           user.Name,
			Email:          user.Email,
			OrganisationID: user.OrganisationID,
		}, "access_token": accessToken, "error": nil})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "existing_user": true, "user": nil, "access_token": nil, "error": nil})
}

func (controller *AuthController) SignIn(c *gin.Context) {
	var userSignInRequest request.UserSignInRequest
	if err := c.ShouldBindJSON(&userSignInRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var existingUser, err = controller.userService.GetUserByEmail(userSignInRequest.Email)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "user": nil, "error": err.Error()})
		fmt.Println("Error occurred while fetching user : ", userSignInRequest.Email, err)
		return
	}

	if existingUser == nil || !(controller.userService.VerifyUserPassword(userSignInRequest.Password, existingUser.Password)) {
		c.JSON(http.StatusOK, gin.H{"success": false, "user": nil, "error": "Invalid Credentials"})
		return
	}

	var accessToken, _ = controller.jwtService.GenerateToken(int(existingUser.ID), existingUser.Email)
	c.JSON(http.StatusOK, gin.H{"success": true, "user": &response.UserResponse{
		Id:             existingUser.ID,
		Name:           existingUser.Name,
		Email:          existingUser.Email,
		OrganisationID: existingUser.OrganisationID,
	}, "access_token": accessToken, "error": nil})
}

func (controller *AuthController) CheckUser(c *gin.Context) {
	var email = c.Query("user_email")
	var existingUser, err = controller.userService.GetUserByEmail(email)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "user_exists": false, "error": err.Error()})
		return
	}

	if existingUser != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "user_exists": true, "error": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "user_exists": false, "error": nil})
}

func NewAuthController(
	githubOauthService *services.GithubOauthService,
	authService *services.AuthService,
	jwtService *services.JWTService,
	userService *services.UserService,
	clientID string,
	clientSecret string,
	redirectURL string,
) *AuthController {
	return &AuthController{
		githubOauthService: githubOauthService,
		authService:        authService,
		jwtService:         jwtService,
		userService:        userService,
		clientID:           clientID,
		clientSecret:       clientSecret,
		redirectURL:        redirectURL,
	}
}
