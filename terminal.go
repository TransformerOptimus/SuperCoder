package main

import (
	"ai-developer/app/config"
	"ai-developer/app/controllers"
	"ai-developer/app/middleware"
	"ai-developer/app/services"
	"context"
	"fmt"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/knadh/koanf/v2"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"net/http"
	"time"
)

func main() {

	config.InitLogger()

	c := dig.New()

	appConfig, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	_ = c.Provide(func() *zap.Logger {
		return config.Logger
	})
	_ = c.Provide(func() *koanf.Koanf {
		return appConfig
	})
	_ = c.Provide(func() *http.Client {
		return &http.Client{}
	})
	//Provide Context
	_ = c.Provide(func() context.Context {
		return context.Background()
	})

	err = c.Provide(func() string {
		return config.JWTSecret()
	})
	if err != nil {
		panic(err)
	}
	err = c.Provide(func() time.Duration {
		return config.JWTExpiryHours()
	})
	if err != nil {
		panic(err)
	}
	err = c.Provide(func(secretKey string, jwtExpiryHours time.Duration) *services.JWTService {
		return services.NewJwtService(secretKey, jwtExpiryHours)
	})
	if err != nil {
		panic(err)
	}
	err = c.Provide(func(secretKey string) *middleware.JWTClaims {
		return middleware.NewJWTClaims(secretKey)
	})
	if err != nil {
		panic(err)
	}

	err = c.Provide(func() *controllers.HealthController {
		return controllers.NewHealth()
	})
	if err != nil {
		fmt.Printf("Error providing HealthController: %v\n", err)
		return
	}

	err = c.Provide(func() *controllers.TerminalController {
		return controllers.NewTerminalController(config.Logger, "bash", []string{}, []string{"localhost"})
	})
	if err != nil {
		fmt.Printf("Error providing TerminalController: %v\n", err)
		return
	}

	// Setup routes and start the server
	err = c.Invoke(func(
		health *controllers.HealthController,
		teminalController *controllers.TerminalController,
		logger *zap.Logger,
		middleware *middleware.JWTClaims,
	) error {

		r := gin.Default()

		r.Use(ginzap.Ginzap(logger, time.RFC3339, true))
		r.Use(ginzap.RecoveryWithZap(logger, true))

		r.Use(TerminalGinMiddleware("http://localhost:3000, https://developer.superagi.com"))
		r.RedirectTrailingSlash = false

		api := r.Group("/api")
		api.GET("/health", health.Health)
		api.GET("/terminal", middleware.AuthenticateJWT(), teminalController.NewTerminal)
		fmt.Println("Starting Gin server on port 8080...")
		return r.Run()
	})

	if err != nil {
		fmt.Println("Error starting server:", err)
		panic(err)
	}
}

func TerminalGinMiddleware(allowOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Request.Header.Del("Origin")

		c.Next()
	}
}
