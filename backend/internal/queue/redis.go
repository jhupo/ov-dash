package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"ov-dash/backend/internal/config"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	redis  *redis.Client
	prefix string
}

func Open(ctx context.Context, cfg config.RedisConfig) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	wrapped := &Client{redis: client, prefix: cfg.Prefix}
	if err := wrapped.Ping(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return wrapped, nil
}

func (c *Client) Close() error {
	return c.redis.Close()
}

func (c *Client) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.redis.Ping(pingCtx).Err()
}

func (c *Client) Enqueue(ctx context.Context, queueName string, job Job) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return c.redis.LPush(ctx, c.key(queueName), payload).Err()
}

func (c *Client) Dequeue(ctx context.Context, queueName string, timeout time.Duration) (*Job, error) {
	result, err := c.redis.BRPop(ctx, timeout, c.key(queueName)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	if len(result) != 2 {
		return nil, fmt.Errorf("unexpected redis BRPOP result length: %d", len(result))
	}

	var job Job
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func (c *Client) Redis() *redis.Client {
	return c.redis
}

func (c *Client) Key(name string) string {
	return c.key(name)
}

func (c *Client) key(name string) string {
	return fmt.Sprintf("%s:%s", c.prefix, name)
}
