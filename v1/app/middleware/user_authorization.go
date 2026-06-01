package middleware

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"strconv"
)

type UserAuthorizationMiddleware struct {
	logger *zap.Logger
}

func (middleware *UserAuthorizationMiddleware) Authorize() gin.HandlerFunc {
	return func(c *gin.Context) {
		authUserId := c.GetUint("user_id")
		if authUserId == 0 {
			c.AbortWithStatusJSON(401, gin.H{"error": "Unauthorized"})
			return
		}
		userId, err := strconv.ParseUint(c.Param("userId"), 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(400, gin.H{"error": "Invalid user ID"})
			return
		}
		if uint(userId) != authUserId {
			c.AbortWithStatusJSON(401, gin.H{"error": "Unauthorized"})
			return
		}
	}
}

func NewUserAuthorizationMiddleware(logger *zap.Logger) *UserAuthorizationMiddleware {
	return &UserAuthorizationMiddleware{
		logger: logger.Named("UserAuthorizationMiddleware"),
	}
}
