package controllers

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/services/auth"
	"ai-developer/app/types/request"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	oauthGithub "golang.org/x/oauth2/github"
	"gorm.io/gorm"
	"net/http"
)

type AuthController struct {
	logger         *zap.Logger
	userService    *services.UserService
	authMiddleware *auth.JWTAuthenticationMiddleware
	envConfig      *config.EnvConfig
	authConfig     *config.GithubOAuthConfig
}

func (controller *AuthController) GithubSignIn(c *gin.Context) {
	if controller.envConfig.IsDevelopment() {
		controller.HandleDefaultUser(c)
		return
	}

	var githubOauthConfig = &oauth2.Config{
		RedirectURL:  controller.authConfig.RedirectURL(),
		ClientID:     controller.authConfig.ClientId(),
		ClientSecret: controller.authConfig.ClientSecret(),
		Scopes:       []string{"user:email"},
		Endpoint:     oauthGithub.Endpoint,
	}
	callback := githubOauthConfig.AuthCodeURL("state", oauth2.AccessTypeOnline)
	c.Redirect(http.StatusTemporaryRedirect, callback)
}

func (controller *AuthController) HandleDefaultUser(c *gin.Context) {
	fmt.Println("Handling Skip Authentication Token.....")
	defaultUser, err := controller.userService.GetDefaultUser()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to get default user"})
		return
	}

	err = controller.authMiddleware.SetAuth(c, defaultUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to set auth"})
		return
	}

	c.Redirect(http.StatusFound, controller.authConfig.FrontendURL())
}

func (controller *AuthController) SignUp(c *gin.Context) {
	var createUserRequest request.CreateUserRequest

	controller.logger.Debug("Creating new user", zap.Any("request", createUserRequest))
	if err := c.ShouldBindJSON(&createUserRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := controller.userService.GetUserByEmail(createUserRequest.Email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to get user"})
		return
	}

	if user == nil {
		user, err = controller.userService.HandleUserSignUp(createUserRequest)
		if err != nil {
			c.JSON(
				http.StatusInternalServerError,
				gin.H{
					"error": err.Error(),
				},
			)
			return
		}
	}
	err = controller.authMiddleware.SetAuth(c, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to set auth"})
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
		},
	)
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
	logger *zap.Logger,
	authMiddleware *auth.JWTAuthenticationMiddleware,
	userService *services.UserService,
	envConfig *config.EnvConfig,
	authConfig *config.GithubOAuthConfig,
) *AuthController {
	return &AuthController{
		logger:         logger.Named("AuthController"),
		authMiddleware: authMiddleware,
		userService:    userService,
		envConfig:      envConfig,
		authConfig:     authConfig,
	}
}
