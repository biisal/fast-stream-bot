package redis

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisService interface {
	Get(ctx context.Context, key string) []byte
	Set(ctx context.Context, key string, value any, ttl time.Duration)
	Del(ctx context.Context, key string)
}

type svc struct {
	r *redis.Client
}

func New(ctx context.Context, connString string) (*redis.Client, RedisService, error) {
	opt, err := redis.ParseURL(connString)
	if err != nil {
		return nil, nil, err
	}

	client := redis.NewClient(opt)
	err = client.Ping(ctx).Err()
	if err != nil {
		defer client.Close()
		return nil, nil, err
	}

	return client, &svc{
		r: client,
	}, nil
}

func (c *svc) Get(ctx context.Context, key string) []byte {
	b, err := c.r.Get(ctx, key).Bytes()
	if err != nil && err != redis.Nil {
		slog.Error("Failed to get data from redis", "error", err)
		return nil
	}
	return b
}

func (c *svc) Set(ctx context.Context, key string, value any, ttl time.Duration) {
	data, err := json.Marshal(value)
	if err != nil {
		slog.Error("Failed to marshal data", "error", err)
		return
	}
	if err := c.r.Set(ctx, key, data, ttl).Err(); err != nil {
		slog.Error("Failed to set data in redis", "error", err)
	}
}

func (c *svc) Del(ctx context.Context, key string) {
	if err := c.r.Del(ctx, key).Err(); err != nil {
		slog.Error("Failed to delete data from redis", "error", err)
	}
}
