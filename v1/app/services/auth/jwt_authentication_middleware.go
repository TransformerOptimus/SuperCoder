package auth

import (
	"ai-developer/app/models"
	ginJwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type JWTAuthenticationMiddleware struct {
	*ginJwt.GinJWTMiddleware
	logger *zap.Logger
}

func (mw *JWTAuthenticationMiddleware) SetAuth(c *gin.Context, user *models.User) error {
	tokenString, _, err := mw.TokenGenerator(user)
	if err != nil {
		mw.logger.Error("Error while generating token", zap.Error(err))
		return err
	}
	if mw.SendCookie {
		expireCookie := mw.TimeFunc().Add(mw.CookieMaxAge)
		maxage := int(expireCookie.Unix() - mw.TimeFunc().Unix())
		if mw.CookieSameSite != 0 {
			c.SetSameSite(mw.CookieSameSite)
		}
		c.SetCookie(
			mw.CookieName,
			tokenString,
			maxage,
			"/",
			mw.CookieDomain,
			mw.SecureCookie,
			mw.CookieHTTPOnly,
		)
	}
	return nil
}

func NewAuthMiddleWare(
	authenticator *Authenticator,
	logger *zap.Logger,
) (*JWTAuthenticationMiddleware, error) {
	middleware, err := ginJwt.New(authenticator.Middleware())
	if err != nil {
		return nil, err
	}
	return &JWTAuthenticationMiddleware{
		GinJWTMiddleware: middleware,
		logger:           logger,
	}, err
}
