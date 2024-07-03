package repositories

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"time"
)

type ProjectConnectionsRepository struct {
	client *redis.Client
	ctx    context.Context
	logger *zap.Logger
}

func NewProjectConnectionsRepository(client *redis.Client, ctx context.Context, logger *zap.Logger) *ProjectConnectionsRepository {
	logger.Info("Creating new ProjectConnectionsRepository.....")
	return &ProjectConnectionsRepository{
		client: client,
		ctx:    ctx,
		logger: logger.Named("ProjectConnectionsRepository"),
	}
}
func getRedisProjectKey(projectID string) string {
	return fmt.Sprintf("project_id:%s", projectID)
}

func (r *ProjectConnectionsRepository) IncrementActiveCount(projectID string, ttl time.Duration) (int64, error) {
	key := getRedisProjectKey(projectID)
	newCount, err := r.client.HIncrBy(r.ctx, key, "active_count", 1).Result()
	if err != nil {
		r.logger.Error("Failed to increment active count", zap.Error(err))
		return 0, err
	}
	_, err = r.client.HSet(r.ctx, key, "last_active_timestamp", time.Now().UTC().Unix()).Result()
	if err != nil {
		r.logger.Error("Failed to set last active timestamp", zap.Error(err))
		return 0, err
	}
	_, err = r.client.Expire(r.ctx, key, ttl).Result()
	if err != nil {
		r.logger.Error("Failed to set TTL", zap.Error(err))
		return 0, err
	}
	return newCount, nil
}

func (r *ProjectConnectionsRepository) DecrementActiveCount(projectID string, ttl time.Duration) (int64, error) {
	key := getRedisProjectKey(projectID)
	newCount, err := r.client.HIncrBy(r.ctx, key, "active_count", -1).Result()
	if err != nil {
		r.logger.Error("Failed to decrement active count", zap.Error(err))
		return 0, err
	}
	if newCount < 0 {
		_, err = r.client.HSet(r.ctx, key, "active_count", 0).Result()
		if err != nil {
			r.logger.Error("Failed to reset negative active count", zap.Error(err))
			return 0, err
		}
		newCount = 0
	}
	_, err = r.client.HSet(r.ctx, key, "last_active_timestamp", time.Now().UTC().Unix()).Result()
	if err != nil {
		r.logger.Error("Failed to set last active timestamp", zap.Error(err))
		return 0, err
	}
	_, err = r.client.Expire(r.ctx, key, ttl).Result()
	if err != nil {
		r.logger.Error("Failed to set TTL", zap.Error(err))
		return 0, err
	}
	return newCount, nil
}

func (r *ProjectConnectionsRepository) GetProjectData(projectID string) (map[string]string, error) {
	return r.client.HGetAll(r.ctx, getRedisProjectKey(projectID)).Result()
}
