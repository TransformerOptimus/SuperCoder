package controllers

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/services/auth"
	"ai-developer/app/types/request"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
)

type AuthController struct {
	logger            *zap.Logger
	userService       *services.UserService
	authMiddleware    *auth.JWTAuthenticationMiddleware
	envConfig         *config.EnvConfig
	githubOAuthConfig *config.GithubOAuthConfig
	githubAuthService *auth.GithubAuthService
	emailAuthService  *auth.EmailAuthService
}

func (controller *AuthController) GithubSignIn(c *gin.Context) {
	if controller.envConfig.IsDevelopment() {
		controller.HandleDefaultUser(c)
		return
	}

	redirectUrl := controller.githubAuthService.GetRedirectUrl("state")
	c.Redirect(http.StatusTemporaryRedirect, redirectUrl)
}

func (controller *AuthController) GithubCallback(c *gin.Context) {
	state := c.Query("state")
	code := c.Query("code")
	user, err := controller.githubAuthService.HandleGithubCallback(code, state)
	if err != nil {
		return
	}
	_ = controller.authMiddleware.SetAuth(c, user)
	c.Redirect(http.StatusTemporaryRedirect, controller.githubOAuthConfig.FrontendURL())
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

	c.Redirect(http.StatusFound, controller.githubOAuthConfig.FrontendURL())
}

func (controller *AuthController) SignIn(c *gin.Context) {
	var userSignInRequest request.UserSignInRequest
	err := c.ShouldBindJSON(&userSignInRequest)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := controller.emailAuthService.HandleSignIn(userSignInRequest.Email, userSignInRequest.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
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
	return
}

func (controller *AuthController) SignUp(c *gin.Context) {
	var createUserRequest request.CreateUserRequest

	controller.logger.Debug("Creating new user", zap.Any("request", createUserRequest))
	if err := c.ShouldBindJSON(&createUserRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := controller.userService.HandleUserSignUp(createUserRequest.Email, createUserRequest.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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
	githubAuthService *auth.GithubAuthService,
	emailAuthService *auth.EmailAuthService,
	userService *services.UserService,
	envConfig *config.EnvConfig,
	githubOAuthConfig *config.GithubOAuthConfig,
) *AuthController {
	return &AuthController{
		logger:            logger.Named("AuthController"),
		authMiddleware:    authMiddleware,
		userService:       userService,
		envConfig:         envConfig,
		githubOAuthConfig: githubOAuthConfig,
		githubAuthService: githubAuthService,
		emailAuthService:  emailAuthService,
	}
}
