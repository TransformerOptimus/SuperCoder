package main

import (
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/gateway/config"
	"github.com/TransformerOptimus/SuperCoder/services/gateway/controllers"
	"github.com/TransformerOptimus/SuperCoder/services/gateway/services"
	"github.com/TransformerOptimus/SuperCoder/services/gateway/services/impl"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}
	defer logger.Sync()

	// Config from env vars: GW_<key> where "__" maps to ".".
	// e.g. GW_gateway__defaults__provider=anthropic
	//      GW_gateway__providers__anthropic__adapter=anthropic
	//      GW_gateway__providers__anthropic__base__url=https://api.anthropic.com
	//      GW_gateway__providers__anthropic__api__key=sk-...
	k := koanf.New(".")
	if err := k.Load(env.Provider("GW_", ".", func(s string) string {
		return strings.ReplaceAll(strings.TrimPrefix(s, "GW_"), "__", ".")
	}), nil); err != nil {
		logger.Fatal("failed to load config from env", zap.Error(err))
	}
	cfg := config.NewGatewayConfig(k)

	// Build the model→provider router from configured providers.
	router := services.NewRouter(logger, cfg.DefaultProvider())
	for _, p := range cfg.Providers() {
		var adapter services.ProviderAdapter
		switch p.Adapter {
		case "anthropic":
			adapter = impl.NewAnthropicAdapter(p.BaseURL, p.Version, p.DefaultMaxTokens, p.APIKey, logger)
		case "openai-responses":
			adapter = impl.NewOpenAIResponsesAdapter(p.BaseURL, p.APIKey, logger)
		default: // "openai-compat" and unknown adapters
			adapter = impl.NewOpenAICompatAdapter(p.Name, p.BaseURL, p.APIKey, p.Models, logger)
		}
		router.RegisterProvider(p.Name, adapter, p.Models)
	}

	health := controllers.NewGatewayHealthController()
	completions := controllers.NewCompletionsController(router, logger)
	models := controllers.NewModelsController(cfg)

	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/api/health", health.Health)
	r.POST("/api/v1/chat/completions", completions.HandleCompletion)
	r.GET("/api/v1/models", models.ListModels)

	addr := os.Getenv("GATEWAY_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	logger.Info("gateway listening", zap.String("addr", addr))
	if err := r.Run(addr); err != nil {
		logger.Fatal("server exited", zap.Error(err))
	}
}
