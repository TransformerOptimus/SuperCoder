package crossorigin

import (
	"strings"
	"time"

	"github.com/gin-contrib/cors"
)

func AllowAllOrigins() cors.Config {
	return cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Auth-Token", "X-Api-Key", "X-Workspace-Id", "X-User-Id", "X-CSRF-Token", "X-XSRF-TOKEN", "X-Session-ID"},
		ExposeHeaders:    []string{"Content-Length"},
		MaxAge:           12 * time.Hour,
		AllowCredentials: false,
	}
}

func AllowSpecificOrigins() cors.Config {
	return cors.Config{
		AllowOriginFunc: func(origin string) bool {
			// Allow localhost for development
			if origin == "http://localhost:3000" || origin == "http://localhost:3001" || origin == "http://localhost:5173" || origin == "http://localhost:5175" {
				return true
			}
			// Allow any ngrok URL (HTTPS only)
			if strings.HasPrefix(origin, "https://") && strings.HasSuffix(origin, ".ngrok-free.app") {
				return true
			}
			// Allow SuperAGI subdomains (HTTPS only)
			if strings.HasPrefix(origin, "https://") && strings.HasSuffix(origin, ".superagi.com") {
				return true
			}
			return false
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Auth-Token", "X-Api-Key", "X-Workspace-Id", "X-User-Id", "X-CSRF-Token", "X-XSRF-TOKEN", "X-Session-ID"},
		ExposeHeaders:    []string{"Content-Length"},
		MaxAge:           12 * time.Hour,
		AllowCredentials: true,
	}
}
