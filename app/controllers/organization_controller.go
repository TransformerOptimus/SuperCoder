package controllers

import (
	"ai-developer/app/services"
	"github.com/gin-gonic/gin"
	"net/http"
)

type OrganizationController struct {
	jwtService          *services.JWTService
	userService         *services.UserService
	organizationService *services.OrganisationService
	redirectUrl         string
}

func (controller *OrganizationController) FetchOrganizationUsers(c *gin.Context) {
	var organizationId = c.GetInt("organization_id")
	var users, err = controller.organizationService.GetOrganizationUsers(uint(organizationId))

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "users": nil, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "users": users, "error": nil})
}

func NewOrganizationController(
	jwtService *services.JWTService,
	userService *services.UserService,
	organizationService *services.OrganisationService,
	redirectUrl string,
) *OrganizationController {
	return &OrganizationController{
		jwtService:          jwtService,
		userService:         userService,
		organizationService: organizationService,
		redirectUrl:         redirectUrl,
	}
}
