package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/dig"
	"go.uber.org/zap"

	"github.com/gin-contrib/cors"
	ginzap "github.com/gin-contrib/zap"
	"github.com/hibiken/asynqmon"

	apicontext "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/api/context"
	coreconfig "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/core"
	logging "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/logging"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/controllers"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/injection"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

func main() {
	logger, _ := logging.NewLogger()
	defer logger.Sync()

	container, err := injection.NewServerContainer(logger)
	if err != nil {
		logger.Fatal("failed to create container", zap.Error(err))
	}

	err = container.Invoke(initialiseServer)
	if err != nil {
		logger.Fatal("failed to initialise server", zap.Error(err))
	}

	logger.Info("server started")
}

type serverParams struct {
	dig.In
	Logger                      *zap.Logger
	ServiceConfig               *coreconfig.ServiceConfig
	IndexerConfig               config.IndexerConfig
	HealthController            *controllers.HealthController
	AsynqmonHandler             *asynqmon.HTTPHandler
	CorsConfig                  cors.Config
	IndexRetrievalController    *controllers.IndexRetrievalController    `optional:"true"`
	IndexDiffController         *controllers.IndexDiffController         `optional:"true"`
	IndexStreamController       *controllers.IndexStreamController       `optional:"true"`
	IndexSyncCompleteController *controllers.IndexSyncCompleteController `optional:"true"`
	SyncSessionService          services.SyncSessionService              `optional:"true"`
	OutboxDispatcher            services.OutboxDispatcher                `optional:"true"`
}

func initialiseServer(p serverParams) {
	r := gin.New()

	// Middleware
	{
		r.Use(cors.New(p.CorsConfig))
		r.Use(ginzap.GinzapWithConfig(p.Logger, &ginzap.Config{
			UTC:          true,
			TimeFormat:   time.RFC3339,
			DefaultLevel: zap.InfoLevel,
		}))
		r.Use(ginzap.RecoveryWithZap(p.Logger, true))
	}

	// Routes
	{
		r.GET(p.AsynqmonHandler.RootPath()+"/*a", gin.WrapH(p.AsynqmonHandler))
		r.GET("/api/health", apicontext.WithApiContext(p.HealthController.Health))

		if p.IndexRetrievalController != nil {
			p.Logger.Info("Indexer enabled, registering indexer routes")
			api := r.Group("/api/v1")
			{
				api.POST("/search", apicontext.WithApiContext(p.IndexRetrievalController.Search))
				api.POST("/graph/query", apicontext.WithApiContext(p.IndexRetrievalController.GraphQuery))
				api.POST("/context", apicontext.WithApiContext(p.IndexRetrievalController.GetContext))
				api.GET("/index/status", apicontext.WithApiContext(p.IndexRetrievalController.GetIndexStatus))
				api.DELETE("/index", apicontext.WithApiContext(p.IndexRetrievalController.DeleteIndex))

				// v3 streaming sync endpoints, gated behind indexer.streaming.enabled.
				if p.IndexerConfig.StreamingEnabled() &&
					p.IndexDiffController != nil &&
					p.IndexStreamController != nil &&
					p.IndexSyncCompleteController != nil {
					api.POST("/index/diff", apicontext.WithApiContext(p.IndexDiffController.Diff))
					api.POST("/index/stream", apicontext.WithApiContext(p.IndexStreamController.Stream))
					api.POST("/index/sync-complete", apicontext.WithApiContext(p.IndexSyncCompleteController.SyncComplete))
					p.Logger.Info("Streaming sync endpoints registered",
						zap.Strings("routes", []string{"/api/v1/index/diff", "/api/v1/index/stream", "/api/v1/index/sync-complete"}))
				} else {
					p.Logger.Info("Streaming sync disabled, /diff /stream /sync-complete routes not registered")
				}
			}
		} else {
			p.Logger.Info("Indexer disabled, indexer routes not registered")
		}
	}

	// Two contexts: signalCtx fires on SIGINT/SIGTERM to trigger the HTTP
	// drain. bgCtx is a separate child cancelled AFTER the drain completes,
	// so the outbox dispatcher stays alive to process outbox rows written
	// by in-flight /stream requests during the drain window.
	signalCtx, signalCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer signalCancel()
	bgCtx, bgCancel := context.WithCancel(context.Background())

	if p.IndexerConfig.StreamingEnabled() &&
		p.OutboxDispatcher != nil &&
		p.SyncSessionService != nil {
		go func() {
			if err := p.OutboxDispatcher.Run(bgCtx); err != nil && !errors.Is(err, context.Canceled) {
				p.Logger.Error("outbox dispatcher exited", zap.Error(err))
			}
		}()
		p.Logger.Info("Outbox dispatcher goroutine started")

		go p.SyncSessionService.RunTTLGCLoop(bgCtx, 1*time.Minute)
		p.Logger.Info("sync_sessions TTL GC goroutine started")
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", p.ServiceConfig.Port()),
		Handler: r,
	}
	go func() {
		<-signalCtx.Done()
		drainCtx, drainCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer drainCancel()
		if err := srv.Shutdown(drainCtx); err != nil {
			p.Logger.Error("http drain failed", zap.Error(err))
		}
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		p.Logger.Fatal("server exited", zap.Error(err))
	}
	// HTTP drain complete — now cancel background goroutines.
	bgCancel()
}
