package utils

import (
	"errors"
	"github.com/gin-gonic/gin"
	"strings"
)

// ExtractBearerToken extracts the bearer token from the request header.
func ExtractBearerToken(c *gin.Context) (string, error) {
	tokenString := c.GetHeader("Authorization")
	if len(tokenString) > len("Bearer ") && strings.HasPrefix(tokenString, "Bearer ") {
		return tokenString[len("Bearer "):], nil
	}

	cookieHeader := c.Request.Header.Get("Cookie")
	accessToken := ""
	cookies := strings.Split(cookieHeader, "; ")
	for _, cookie := range cookies {
		parts := strings.SplitN(cookie, "=", 2)
		if len(parts) == 2 && parts[0] == "accessToken" {
			accessToken = parts[1]
			break
		}
	}

	if accessToken != "" {
		return accessToken, nil
	}

	return "", errors.New("Bearer token required")
}
