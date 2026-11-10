package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	rdb *redis.Client
}

func NewRedisClient(cfg *Config) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Url, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		Protocol: cfg.Redis.Protocol,
	})
	return &RedisClient{
		rdb: rdb,
	}, nil
}

func (r *RedisClient) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	if err := r.rdb.Set(ctx, key, val, ttl); err != nil {
		return err.Err()
	}
	return nil
}

func (r *RedisClient) Get(ctx context.Context, key string) (interface{}, error) {
	val, err := r.rdb.Get(ctx, key).Result()
	if err != nil {
		return val, err
	}
	return nil, err
}
