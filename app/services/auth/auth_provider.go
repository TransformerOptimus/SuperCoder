package auth

import (
	"github.com/gin-gonic/gin"
)

type AuthProvider interface {
	Authenticate(c *gin.Context) (interface{}, error)
}
