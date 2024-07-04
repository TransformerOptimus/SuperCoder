package controllers

import (
	"ai-developer/app/config"
	"ai-developer/app/constants"
	"ai-developer/app/services"
	"fmt"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	oauthGithub "golang.org/x/oauth2/github"
	"net/http"
	"net/url"
	"strconv"
)

type OauthController struct {
	authService        *services.AuthService
	githubOauthService *services.GithubOauthService
	clientID           string
	clientSecret       string
	redirectURL        string
}

func (controller *OauthController) GithubSignIn(c *gin.Context) {
	var env = config.Get("app.env")
	fmt.Println("ENV : ", env)
	if env == constants.Development {
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

func (controller *OauthController) GithubCallback(c *gin.Context) {
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

func NewOauthController(
	githubOauthService *services.GithubOauthService,
	authService *services.AuthService,
	clientID string,
	clientSecret string,
	redirectURL string,
) *OauthController {
	return &OauthController{
		githubOauthService: githubOauthService,
		authService:        authService,
		clientID:           clientID,
		clientSecret:       clientSecret,
		redirectURL:        redirectURL,
	}
}
