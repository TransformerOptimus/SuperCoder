package main

import (
	"github.com/gin-gonic/gin"
	"github.com/knadh/koanf/v2"
	"github.com/newrelic/go-agent/v3/newrelic"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"log"
	"workspace-service/app/clients"
	"workspace-service/app/config"
	"workspace-service/app/controllers"
	"workspace-service/app/services"
	"workspace-service/app/services/impl"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Printf("Failed to provide logger: %v", err)
		panic(err)
	}
	logger.Info("Starting workspace service")

	devContainer := dig.New()
	prodContainer := dig.New()

	appConfig, err := config.LoadConfig()
	if err != nil {
		logger.Error("Failed to load config", zap.Error(err))
		panic(err)
	}

	envConfig := config.NewEnvConfig(appConfig)

	var container *dig.Container
	if envConfig.IsDev() {
		container = devContainer
		logger.Info("Running in development mode")
	} else {
		container = prodContainer
		logger.Info("Running in production mode")
	}

	_ = container.Provide(func() *zap.Logger {
		return logger
	})

	err = container.Provide(func() *koanf.Koanf {
		return appConfig
	})
	if err != nil {
		logger.Error("Failed to provide config", zap.Error(err))
		panic(err)
	}
	logger.Info("Config loaded")

	err = container.Provide(config.NewEnvConfig)
	if err != nil {
		logger.Error("Failed to provide env config", zap.Error(err))
		panic(err)
	}

	logger.Info("Env config loaded")

	err = container.Provide(config.NewNewRelicConfig)
	if err != nil {
		logger.Error("Failed to provide new relic config", zap.Error(err))
		panic(err)
	}

	_ = container.Provide(func(
		config *config.NewRelicConfig,
	) *newrelic.Application {
		nrApp, err := newrelic.NewApplication(
			newrelic.ConfigAppName(config.AppName()),
			newrelic.ConfigLicense(config.LicenseKey()),
			newrelic.ConfigDistributedTracerEnabled(true),
		)
		if err != nil {
			logger.Error("Failed to create new relic application", zap.Error(err))
			return nil
		}
		logger.Info("New relic application created")
		return nrApp
	})

	_ = container.Provide(config.NewWorkspaceJobs)

	_ = container.Provide(config.NewWorkspaceService)

	err = devContainer.Provide(clients.NewDockerClient)
	if err != nil {
		logger.Error("Failed to provide docker client", zap.Error(err))
		return
	}

	err = devContainer.Provide(func(logger *zap.Logger,
		workspaceServiceConfig *config.WorkspaceServiceConfig) services.WorkspaceService {
		logger.Info("Providing docker workspace service")
		return impl.NewDockerWorkspaceService(logger, workspaceServiceConfig)
	})
	if err != nil {
		logger.Error("Failed to provide docker workspace service", zap.Error(err))
		return
	}

	err = devContainer.Provide(impl.NewDockerJobService)
	if err != nil {
		logger.Error("Failed to provide docker job service", zap.Error(err))
		return
	}

	err = prodContainer.Provide(clients.NewK8sControllerClient)
	if err != nil {
		logger.Error("Failed to provide k8s controller client", zap.Error(err))
		return
	}

	err = prodContainer.Provide(clients.NewK8sClientSet)
	if err != nil {
		logger.Error("Failed to provide k8s client", zap.Error(err))
		return
	}

	err = prodContainer.Provide(impl.NewK8sWorkspaceService)
	if err != nil {
		logger.Error("Failed to provide k8s workspace service", zap.Error(err))
		return
	}

	err = prodContainer.Provide(impl.NewK8sJobService)
	if err != nil {
		logger.Error("Failed to provide k8s workspace service", zap.Error(err))
		return
	}

	_ = container.Provide(controllers.NewHealthController)
	_ = container.Provide(controllers.NewWorkspaceController)
	_ = container.Provide(controllers.NewJobsController)

	if err != nil {
		logger.Error("Failed to provide workspace controller", zap.Error(err))
		panic(err)
	}

	err = container.Invoke(func(
		logger *zap.Logger,
		wsController *controllers.WorkspaceController,
		healthController *controllers.HealthController,
		jobsController *controllers.JobsController,
		nrApp *newrelic.Application,
	) (err error) {
		defer func() {
			err := logger.Sync()
			if err != nil {
				log.Printf("Failed to sync logger: %v", err)
				return
			}
		}()

		r := gin.Default()
		// Add New Relic middleware to the Gin router
		r.Use(func(c *gin.Context) {
			txn := nrApp.StartTransaction(c.FullPath())
			defer txn.End()
			c.Set("newRelicTransaction", txn)
			c.Next()
		})
		r.Handle("GET", "/api/health", healthController.Health)
		r.Handle("POST", "/api/v1/workspaces", wsController.CreateWorkspace)
		r.Handle("DELETE", "/api/v1/workspaces/:workspaceId", wsController.DeleteWorkspace)

		r.Handle("POST", "/api/v1/jobs", jobsController.CreateWorkspace)
		return r.Run()
	})

	if err != nil {
		logger.Error("Failed to run server", zap.Error(err))
		return
	}
}
