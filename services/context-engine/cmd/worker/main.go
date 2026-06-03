package main

import (
	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	logging "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/logging"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/injection"
)

func main() {
	logger, _ := logging.NewLogger()
	defer logger.Sync()

	container, err := injection.NewWorkerContainer(logger)
	if err != nil {
		logger.Error("failed to create container", zap.Error(err))
		panic(err)
	}

	err = container.Invoke(func(
		server *asynq.Server,
		scheduler *asynq.Scheduler,
		serverMux *asynq.ServeMux,
	) {
		go func() {
			if err = scheduler.Run(); err != nil {
				logger.Error("Failed to start asynq scheduler", zap.Error(err))
				panic(err)
			}
		}()
		defer scheduler.Shutdown()

		go func() {
			if err = server.Run(serverMux); err != nil {
				logger.Error("Failed to start asynq server", zap.Error(err))
				panic(err)
			}
		}()
		defer server.Shutdown()

		select {}
	})
	if err != nil {
		logger.Error("failed to start server", zap.Error(err))
		panic(err)
	}
}
