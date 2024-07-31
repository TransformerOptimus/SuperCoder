package auth

import (
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"errors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type EmailAuthProvider struct {
	AuthProvider
	logger      *zap.Logger
	userService *services.UserService
}

func (eap EmailAuthProvider) Authenticate(c *gin.Context) (user interface{}, err error) {
	var userSignInRequest request.UserSignInRequest
	err = c.ShouldBindJSON(&userSignInRequest)
	if err != nil {
		return
	}
	existingUser, err := eap.userService.GetUserByEmail(userSignInRequest.Email)
	if err != nil {
		return
	}

	if existingUser == nil {
		return nil, errors.New("user not found")
	}

	validated := eap.userService.VerifyUserPassword(userSignInRequest.Password, existingUser.Password)

	if !validated {
		return nil, errors.New("invalid credentials")
	}
	return existingUser, nil
}

func NewEmailAuthProvider(
	logger *zap.Logger,
	userService *services.UserService,
) *EmailAuthProvider {
	return &EmailAuthProvider{
		logger:      logger.Named("EmailAuthProvider"),
		userService: userService,
	}
}
