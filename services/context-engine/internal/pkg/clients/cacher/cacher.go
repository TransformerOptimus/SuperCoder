package cacher_client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-gorm/caches/v4"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	redis_config "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/redis"
)

type redisCacher struct {
	rdb    *redis.Client
	ttl    int
	tracer trace.Tracer
}

func (c *redisCacher) Get(ctx context.Context, key string, q *caches.Query[any]) (*caches.Query[any], error) {
	ctx, span := c.tracer.Start(ctx, "RedisCache.Get")
	defer span.End()

	res, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		span.SetAttributes(attribute.Bool("cache.hit", false))
		return nil, nil
	}

	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	if err := q.Unmarshal([]byte(res)); err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.Bool("cache.hit", true))
	return q, nil
}

func (c *redisCacher) Store(ctx context.Context, key string, val *caches.Query[any]) error {
	ctx, span := c.tracer.Start(ctx, "RedisCache.Store")
	defer span.End()

	res, err := val.Marshal()
	if err != nil {
		span.RecordError(err)
		return err
	}

	err = c.rdb.Set(ctx, key, res, time.Duration(c.ttl)*time.Second).Err()
	if err != nil {
		span.RecordError(err)
		return err
	}

	return nil
}

func (c *redisCacher) Invalidate(ctx context.Context) error {
	ctx, span := c.tracer.Start(ctx, "RedisCache.Invalidate")
	defer span.End()

	var (
		cursor uint64
		keys   []string
	)
	for {
		var (
			k   []string
			err error
		)
		k, cursor, err = c.rdb.Scan(ctx, cursor, fmt.Sprintf("%s*", caches.IdentifierPrefix), 0).Result()
		if err != nil {
			span.RecordError(err)
			return err
		}
		keys = append(keys, k...)
		if cursor == 0 {
			break
		}
	}

	span.SetAttributes(attribute.Int("cache.keys_found", len(keys)))

	if len(keys) > 0 {
		if _, err := c.rdb.Del(ctx, keys...).Result(); err != nil {
			span.RecordError(err)
			return err
		}
	}
	return nil
}

func NewRedisCache(redisConfig redis_config.RedisConfig) *caches.Caches {
	return &caches.Caches{Conf: &caches.Config{
		Cacher: &redisCacher{
			rdb: redis.NewClient(&redis.Options{
				Addr: redisConfig.Addr(),
				DB:   redisConfig.CacheDB(),
			}),
			ttl:    redisConfig.CacheTTL(),
			tracer: otel.Tracer("github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/clients/cacher"),
		},
	}}
}
