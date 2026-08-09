package cache

import (
	"context"
	"fmt"
	"time"

	"ov-dash/backend/internal/config"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	redis  *redis.Client
	prefix string
}

func Open(ctx context.Context, cfg config.RedisConfig) (*Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	cache := &Cache{redis: client, prefix: cfg.Prefix}
	if err := cache.Ping(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return cache, nil
}

func (c *Cache) Close() error {
	return c.redis.Close()
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	return c.redis.Get(ctx, c.key(key)).Result()
}

func (c *Cache) GetDelete(ctx context.Context, key string) (string, error) {
	return c.redis.GetDel(ctx, c.key(key)).Result()
}

func (c *Cache) Take(ctx context.Context, key string) (string, error) {
	return c.GetDelete(ctx, key)
}

func (c *Cache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return c.redis.Set(ctx, c.key(key), value, ttl).Err()
}

func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	namespaced := make([]string, 0, len(keys))
	for _, key := range keys {
		namespaced = append(namespaced, c.key(key))
	}
	return c.redis.Del(ctx, namespaced...).Err()
}

func (c *Cache) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.redis.Ping(pingCtx).Err()
}

func (c *Cache) key(name string) string {
	return fmt.Sprintf("%s:%s", c.prefix, name)
}
