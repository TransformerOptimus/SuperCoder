package services

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type ProjectNotificationService struct {
	client *redis.Client
	ctx    context.Context
	logger *zap.Logger
}

func NewProjectNotificationService(client *redis.Client, ctx context.Context, logger *zap.Logger) *ProjectNotificationService {
	return &ProjectNotificationService{
		client: client,
		ctx:    ctx,
		logger: logger,
	}
}

func (s *ProjectNotificationService) SendNotification(projectID uint, storyID uint, message string) error {
	channel := fmt.Sprintf("%d_%d", projectID, storyID)
    err := s.client.Publish(s.ctx, channel, message).Err()
    if err != nil {
        fmt.Println("Error publishing message to Redis: ", err.Error())
        return err
    }
	return nil
}

func (s *ProjectNotificationService) ReceiveNotification(channelNameFormat string) (*redis.PubSub, error) {
	pubsub := s.client.PSubscribe(s.ctx, channelNameFormat)
	_, err := pubsub.Receive(s.ctx)
	if err != nil {
		s.logger.Error("Error subscribing to channel", zap.Error(err))
		return nil, err
	}
	return pubsub, nil
}