package clients

import (
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func NewK8sClientSet(logger *zap.Logger) (clientset *kubernetes.Clientset, err error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("Failed to get in-cluster config", zap.Error(err))
		return
	}
	clientset, err = kubernetes.NewForConfig(config)
	return
}
