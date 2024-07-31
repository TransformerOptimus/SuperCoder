package auth

import (
	"ai-developer/app/models"
	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
)

type JWTAuthenticationMiddleware struct {
	*jwt.GinJWTMiddleware
}

func (m *JWTAuthenticationMiddleware) SetAuth(c *gin.Context, user *models.User) error {
	tokenString, _, err := m.TokenGenerator(user)
	if err != nil {
		return err
	}
	if m.SendCookie {
		expireCookie := m.TimeFunc().Add(m.CookieMaxAge)
		maxage := int(expireCookie.Unix() - m.TimeFunc().Unix())
		if m.CookieSameSite != 0 {
			c.SetSameSite(m.CookieSameSite)
		}
		c.SetCookie(
			m.CookieName,
			tokenString,
			maxage,
			"/",
			m.CookieDomain,
			m.SecureCookie,
			m.CookieHTTPOnly,
		)
	}
	return nil
}

func NewAuthMiddleWare(authenticator *Authenticator) (*JWTAuthenticationMiddleware, error) {
	middleware, err := jwt.New(authenticator.Middleware())
	if err != nil {
		return nil, err
	}
	return &JWTAuthenticationMiddleware{
		middleware,
	}, err
}
