package clients

import (
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

func NewDockerClient(logger *zap.Logger) (apiClient *client.Client, err error) {
	apiClient, err = client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		logger.Error("Failed to create docker client", zap.Error(err))
		panic(err)
	}
	return
}
