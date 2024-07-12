package controllers

import (
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

type UserController struct {
	jwtService  *services.JWTService
	userService *services.UserService
	redirectUrl string
}

func (controller *UserController) CheckUser(c *gin.Context) {
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

func (controller *UserController) SignUp(c *gin.Context) {
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
		user.Password = ""
		c.JSON(http.StatusOK, gin.H{"success": true, "existing_user": false, "user": user, "access_token": accessToken, "error": nil})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "existing_user": true, "user": nil, "access_token": nil, "error": nil})
}

func (controller *UserController) SignIn(c *gin.Context) {
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

	if existingUser == nil || existingUser.Password != userSignInRequest.Password {
		c.JSON(http.StatusOK, gin.H{"success": false, "user": nil, "error": "Invalid Credentials"})
		return
	}

	var accessToken, _ = controller.jwtService.GenerateToken(int(existingUser.ID), existingUser.Email)

	existingUser.Password = ""
	c.JSON(http.StatusOK, gin.H{"success": true, "user": existingUser, "access_token": accessToken, "error": nil})

}

func NewUserController(
	jwtService *services.JWTService,
	userService *services.UserService,
	redirectUrl string,
) *UserController {
	return &UserController{
		jwtService:  jwtService,
		userService: userService,
		redirectUrl: redirectUrl,
	}
}
