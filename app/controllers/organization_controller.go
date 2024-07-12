package controllers

import (
	"ai-developer/app/services"
	"ai-developer/app/types/response"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type OrganizationController struct {
	jwtService          *services.JWTService
	userService         *services.UserService
	organizationService *services.OrganisationService
}

func (controller *OrganizationController) FetchOrganizationUsers(c *gin.Context) {
	var users []*response.UserResponse
	var organizationId = c.Param("organisation_id")
	var organizationIdInt, err = strconv.Atoi(organizationId)
	if err != nil {
		c.JSON(http.StatusOK, &response.FetchOrganisationUserResponse{Success: false, Error: "Invalid input for organisation_id", User: nil})
	}

	fmt.Println("Fetching org users: ", organizationId)
	users, err = controller.organizationService.GetOrganizationUsers(uint(organizationIdInt))
	if err != nil {
		c.JSON(http.StatusOK, &response.FetchOrganisationUserResponse{Success: false, Error: err.Error(), User: nil})
		return
	}

	c.JSON(http.StatusOK, &response.FetchOrganisationUserResponse{Success: true, Error: nil, User: users})
}

func NewOrganizationController(
	jwtService *services.JWTService,
	userService *services.UserService,
	organizationService *services.OrganisationService,
) *OrganizationController {
	return &OrganizationController{
		jwtService:          jwtService,
		userService:         userService,
		organizationService: organizationService,
	}
}
