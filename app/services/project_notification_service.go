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

func (s *ProjectNotificationService) SendNotification(projectID uint, message string) error {
	channel := fmt.Sprintf("project-notifications-%d", projectID)
	err := s.client.Publish(s.ctx, channel, message).Err()
	if err != nil {
		s.logger.Error("Error publishing message to Redis", zap.Error(err))
	        return err
	}
	s.logger.Debug("Sent message to", zap.Any(" channel- ",channel), zap.Any(" message- ",message))
	return nil
}

func (s *ProjectNotificationService) ReceiveNotification(sendFunction func(msg string), projectID string, channel string) (*redis.PubSub, error) {
	pubsub := s.client.Subscribe(s.ctx, channel)
	_, err := pubsub.Receive(s.ctx)
	if err != nil {
		s.logger.Error("Error subscribing to channel", zap.Error(err))
		return nil, err
	}

	go func() {
		ch := pubsub.Channel()
		for msg := range ch {
			channel := msg.Channel
			s.logger.Debug("Received message on", zap.Any(" channel- ",channel), zap.Any(" message- ",msg.Payload))
			sendFunction(msg.Payload)
		}
	}()

	return pubsub, nil
}
