package services

import (
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

const (
	lockKeyPrefix = "lock:project:"
)

type RedisLocker struct {
	client *redis.Client
}

func NewRedisLocker(client *redis.Client) *RedisLocker {
	return &RedisLocker{client: client}
}

func (rl *RedisLocker) AcquireLock(projectHashID string, lockTimeout time.Duration) (bool, error) {
	lockKey := lockKeyPrefix + projectHashID
	ok, err := rl.client.SetNX(context.Background(), lockKey, 1, lockTimeout).Result()
	return ok, err
}

func (rl *RedisLocker) ReleaseLock(projectHashID string) error {
	lockKey := lockKeyPrefix + projectHashID
	return rl.client.Del(context.Background(), lockKey).Err()
}
