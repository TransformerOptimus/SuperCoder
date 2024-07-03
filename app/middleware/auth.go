package middleware

import (
	"ai-developer/app/utils"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"net/http"
)

type JWTClaims struct {
	secretKey string
}

func (s *JWTClaims) parseToken(tokenString string) (string, int, error) {
	jwtKey := []byte(s.secretKey)
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil {
		return "", 0, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", 0, fmt.Errorf("invalid token")
	}
	email, ok := claims["email"].(string)
	if !ok {
		return "", 0, fmt.Errorf("email not found in claims")
	}

	userID, ok := claims["user_id"].(float64)
	if !ok {
		return "", 0, fmt.Errorf("user_id not found in claims")
	}

	return email, int(userID), nil
}

// AuthenticateJWT middleware checks the JWT token in the request
func (s *JWTClaims) AuthenticateJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := utils.ExtractBearerToken(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token1"})
			c.Abort()
			return
		}

		email, userId, err := s.parseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		c.Set("email", email)
		c.Set("user_id", userId)
		c.Next()
	}
}

func NewJWTClaims(secretKey string) *JWTClaims {
	return &JWTClaims{
		secretKey: secretKey,
	}
}
