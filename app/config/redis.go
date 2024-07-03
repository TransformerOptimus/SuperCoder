package config

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
)

func RedisHost() string {
	return config.String("redis.host")
}

func RedisPort() int {
	return config.Int("redis.port")
}

func RedisDB() int {
	return config.Int("redis.db")
}

func RedisAddress() string {
	return fmt.Sprintf("%s:%d", RedisHost(), RedisPort())
}

func InitRedis(ctx context.Context) (client *redis.Client) {
	fmt.Println("Initializing Redis Client.....")
	client = redis.NewClient(&redis.Options{
		Addr: RedisAddress(),
		DB:   1,
	})

	output, err := client.Ping(ctx).Result()
	if err != nil {
		fmt.Println("Failed to connect to Redis")
		panic(err)
	}
	fmt.Printf("Connected to Redis: %v\n", output)
	return client
}
