package controllers

import (
	"ai-developer/app/config"
	"ai-developer/app/models"
	"ai-developer/app/repositories"
	"ai-developer/app/services"
	"ai-developer/app/types/request"
	"ai-developer/app/types/response"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
)

type OrganizationController struct {
	jwtService           *services.JWTService
	userService          *services.UserService
	organizationService  *services.OrganisationService
	organisationUserRepo *repositories.OrganisationUserRepository
	appRedirectUrl       string
}

func (controller *OrganizationController) FetchOrganizationUsers(c *gin.Context) {
	var users []*response.UserResponse
	var organizationId = c.Param("organisation_id")
	var organizationIdInt, err = strconv.Atoi(organizationId)
	if err != nil {
		c.JSON(http.StatusBadRequest, &response.FetchOrganisationUserResponse{Success: false, Error: "Invalid input for organisation_id", Users: nil})
	}

	fmt.Println("Fetching org users: ", organizationId)
	users, err = controller.organizationService.GetOrganizationUsers(uint(organizationIdInt))
	if err != nil {
		c.JSON(http.StatusInternalServerError, &response.FetchOrganisationUserResponse{Success: false, Error: err.Error(), Users: nil})
		return
	}

	c.JSON(http.StatusOK, &response.FetchOrganisationUserResponse{Success: true, Error: nil, Users: users})
}

func (controller *OrganizationController) InviteUserToOrganisation(c *gin.Context) {
	var inviteUserRequest request.InviteUserRequest
	if err := c.ShouldBindJSON(&inviteUserRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}
	var organizationId = c.Param("organisation_id")
	var organizationIdInt, err = strconv.Atoi(organizationId)
	if err != nil {
		c.JSON(http.StatusBadRequest, &response.SendEmailResponse{
			Success:   false,
			MessageId: "",
			Error:     err.Error(),
		})
		return
	}
	sendEmailResponse, err := controller.organizationService.InviteUserToOrganization(organizationIdInt, inviteUserRequest.Email, inviteUserRequest.CurrentUserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, &response.SendEmailResponse{
			Success:   false,
			MessageId: "",
			Error:     err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, sendEmailResponse)
}

func (controller *OrganizationController) HandleUserInvite(c *gin.Context) {
	var inviteToken = c.Query("invite_token")
	email, organisationID, err := controller.jwtService.DecodeInviteToken(inviteToken)
	if err != nil {
		redirectUrl := controller.appRedirectUrl + "?error_msg=INVALID_TOKEN"
		c.Redirect(http.StatusTemporaryRedirect, redirectUrl)
		return
	}
	user, err := controller.userService.GetUserByEmail(email)
	if user != nil {
		orgUser, err := controller.organisationUserRepo.CreateOrganisationUser(controller.organisationUserRepo.GetDB(), &models.OrganisationUser{
			OrganisationID: uint(organisationID),
			UserID:         user.ID,
			IsActive:       true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		})
		if err != nil {
			redirectUrl := controller.appRedirectUrl + "?error_msg=SERVER_ERROR"
			c.Redirect(http.StatusTemporaryRedirect, redirectUrl)
			return
		}
		user.OrganisationID = orgUser.OrganisationID
		err = controller.userService.UpdateUserByEmail(user.Email, user)
		if err != nil {
			redirectUrl := controller.appRedirectUrl + "?error_msg=SERVER_ERROR"
			c.Redirect(http.StatusTemporaryRedirect, redirectUrl)
			return
		}
		redirectUrl := controller.appRedirectUrl + "?user_email=" + email + "&invite_token=" + inviteToken
		c.Redirect(http.StatusTemporaryRedirect, redirectUrl)
		return
	}
	redirectUrl := controller.appRedirectUrl + "?user_email=" + email + "&invite_token=" + inviteToken
	c.Redirect(http.StatusTemporaryRedirect, redirectUrl)
}

func (controller *OrganizationController) RemoveUserFromOrganisation(c *gin.Context) {
	var removeOrgUserRequest request.RemoveOrgUserRequest
	if err := c.ShouldBindJSON(&removeOrgUserRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}
	var organizationId = c.Param("organisation_id")
	var organizationIdInt, err = strconv.Atoi(organizationId)
	if err != nil {
		c.JSON(http.StatusBadRequest, &response.FetchOrganisationUserResponse{Success: false, Error: "Invalid input for organisation_id", Users: nil})
		return
	}
	fmt.Println("User : ", removeOrgUserRequest.UserID, uint(organizationIdInt))
	user, err := controller.userService.GetUserByID(uint(removeOrgUserRequest.UserID))
	if user == nil {
		c.JSON(http.StatusBadRequest, &response.FetchOrganisationUserResponse{Success: false, Error: "User not found"})
		return
	}
	organisation := &models.Organisation{
		Name: controller.organizationService.CreateOrganisationName(),
	}
	organisation, err = controller.organizationService.CreateOrganisation(organisation)
	if err != nil {
		fmt.Println("Error while creating organization: ", err.Error())
		c.JSON(http.StatusInternalServerError, &response.FetchOrganisationUserResponse{Success: false, Error: err.Error()})
		return
	}
	user.OrganisationID = organisation.ID
	err = controller.userService.UpdateUserByEmail(user.Email, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &response.FetchOrganisationUserResponse{Success: false, Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, &response.FetchOrganisationUserResponse{Success: true, Error: nil})
}

func NewOrganizationController(
	jwtService *services.JWTService,
	userService *services.UserService,
	organizationService *services.OrganisationService,
	organisationUserRepo *repositories.OrganisationUserRepository,
) *OrganizationController {
	return &OrganizationController{
		jwtService:           jwtService,
		userService:          userService,
		organizationService:  organizationService,
		appRedirectUrl:       config.AppUrl(),
		organisationUserRepo: organisationUserRepo,
	}
}
