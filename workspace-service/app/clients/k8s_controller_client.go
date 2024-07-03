package clients

import (
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewK8sControllerClient(logger *zap.Logger) (controllerClient client.Client, err error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("Failed to get in-cluster config", zap.Error(err))
		return
	}
	controllerClient, err = client.New(restConfig, client.Options{})
	return
}
