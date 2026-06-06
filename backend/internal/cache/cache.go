package cache

import (
	"context"
	"time"

	"ov-dash/backend/internal/queue"
)

type Cache struct {
	queue *queue.Client
}

func New(queueClient *queue.Client) *Cache {
	return &Cache{queue: queueClient}
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	return c.queue.Redis().Get(ctx, c.queue.Key(key)).Result()
}

func (c *Cache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return c.queue.Redis().Set(ctx, c.queue.Key(key), value, ttl).Err()
}

func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	namespaced := make([]string, 0, len(keys))
	for _, key := range keys {
		namespaced = append(namespaced, c.queue.Key(key))
	}
	return c.queue.Redis().Del(ctx, namespaced...).Err()
}

func (c *Cache) Ping(ctx context.Context) error {
	return c.queue.Ping(ctx)
}
