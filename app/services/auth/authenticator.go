package auth

import (
	"ai-developer/app/config"
	"ai-developer/app/models"
	"ai-developer/app/services"
	"errors"
	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
	"time"
)

type Authenticator struct {
	jwtConfig          *config.JWTConfig
	userService        *services.UserService
	authService        *services.AuthService
	envConfig          *config.EnvConfig
	githubAuthProvider *GithubAuthProvider
	emailAuthProvider  *EmailAuthProvider
	logger             *zap.Logger
}

const (
	AuthType = "auth_type"
	Github   = "github"
	Email    = "email"
)

func (a *Authenticator) payloadFunc() func(data interface{}) jwt.MapClaims {
	return func(data interface{}) jwt.MapClaims {
		if v, ok := data.(*models.User); ok {
			a.logger.Info("Creating payload", zap.Any("user", v))
			return jwt.MapClaims{
				jwt.IdentityKey: v.ID,
				"user_id":       v.ID,
				"email":         v.Email,
			}
		}
		return jwt.MapClaims{}
	}
}

func (a *Authenticator) identityHandler() func(c *gin.Context) interface{} {
	return func(c *gin.Context) interface{} {
		claims := jwt.ExtractClaims(c)
		a.logger.Info("Extracting claims", zap.Any("claims", claims))
		user, err := a.userService.GetUserByID(uint(claims[jwt.IdentityKey].(float64)))
		if err != nil {
			c.JSON(500, gin.H{
				"error": "User not found",
			})
			return nil
		}
		a.logger.Info("User found", zap.Any("user", user))
		return user
	}
}

func (a *Authenticator) getAuthProvider(authType string) (provider AuthProvider, err error) {
	switch authType {
	case Github:
		provider = a.githubAuthProvider
		return
	case Email:
		provider = a.emailAuthProvider
		return
	}
	return nil, errors.New("invalid auth type")
}

func (a *Authenticator) authenticator() func(c *gin.Context) (interface{}, error) {
	return func(c *gin.Context) (interface{}, error) {
		authType, exists := c.Get(AuthType)
		if !exists {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Auth type not found",
			})
			return nil, nil
		}
		authProvider, err := a.getAuthProvider(authType.(string))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid auth type",
			})
			return nil, nil
		}
		return authProvider.Authenticate(c)
	}
}

func (a *Authenticator) unauthorized() func(c *gin.Context, code int, message string) {
	return func(c *gin.Context, code int, message string) {
		c.JSON(code, gin.H{
			"code":    code,
			"message": message,
		})
	}
}

func (a *Authenticator) GithubAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(AuthType, Github)
	}
}

func (a *Authenticator) EmailAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(AuthType, Email)
	}
}

func (a *Authenticator) Middleware() *jwt.GinJWTMiddleware {
	return &jwt.GinJWTMiddleware{
		Realm:           "supercoder",
		Key:             []byte(a.jwtConfig.Secret()),
		Timeout:         24 * 7 * time.Hour,
		MaxRefresh:      24 * 7 * time.Hour,
		IdentityKey:     jwt.IdentityKey,
		PayloadFunc:     a.payloadFunc(),
		IdentityHandler: a.identityHandler(),
		Authenticator:   a.authenticator(),
		Unauthorized:    a.unauthorized(),
		TokenLookup:     "header: Authorization, query: token, cookie: token",
		TokenHeadName:   "Bearer",
		TimeFunc:        time.Now,
		SendCookie:      true,
		SecureCookie:    !a.envConfig.IsDevelopment(),
		CookieHTTPOnly:  false,
		CookieDomain:    a.envConfig.Domain(),
		CookieName:      "token",
		CookieSameSite:  http.SameSiteLaxMode,
		CookieMaxAge:    (24 * 7 * time.Hour) - 10,
	}
}

func NewAuthenticator(
	jwtConfig *config.JWTConfig,
	authService *services.AuthService,
	userService *services.UserService,
	githubAuthProvider *GithubAuthProvider,
	emailAuthProvider *EmailAuthProvider,
	envConfig *config.EnvConfig,
	logger *zap.Logger,
) *Authenticator {
	return &Authenticator{
		jwtConfig:          jwtConfig,
		userService:        userService,
		authService:        authService,
		envConfig:          envConfig,
		githubAuthProvider: githubAuthProvider,
		emailAuthProvider:  emailAuthProvider,
		logger:             logger.Named("Authenticator"),
	}
}
