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
	audit  AuditStore
}

type AuditStore interface {
	RecordJobEnqueued(ctx context.Context, queueName string, job Job) error
	RecordJobRunning(ctx context.Context, queueName string, job Job) error
	RecordJobCompleted(ctx context.Context, queueName string, job Job) error
	RecordJobFailed(ctx context.Context, queueName string, job Job, message string, nextRunAt *time.Time) error
	RecordJobDead(ctx context.Context, queueName string, job Job, message string) error
}

type Option func(*Client)

func WithAudit(audit AuditStore) Option {
	return func(c *Client) {
		c.audit = audit
	}
}

func Open(ctx context.Context, cfg config.RedisConfig, opts ...Option) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	wrapped := &Client{redis: client, prefix: cfg.Prefix}
	for _, opt := range opts {
		opt(wrapped)
	}
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
	job.Normalize()
	if c.audit != nil {
		if err := c.audit.RecordJobEnqueued(ctx, queueName, job); err != nil {
			return err
		}
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if err := c.redis.LPush(ctx, c.key(queueName), payload).Err(); err != nil {
		if c.audit != nil {
			_ = c.audit.RecordJobFailed(ctx, queueName, job, fmt.Sprintf("enqueue redis: %v", err), nil)
		}
		return err
	}
	return nil
}

func (c *Client) ScheduleRetry(ctx context.Context, queueName string, job Job, runAt time.Time, message string) error {
	job.Normalize()
	if c.audit != nil {
		if err := c.audit.RecordJobFailed(ctx, queueName, job, message, &runAt); err != nil {
			return err
		}
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return c.redis.ZAdd(ctx, c.scheduledKey(queueName), redis.Z{
		Score:  float64(runAt.UnixMilli()),
		Member: payload,
	}).Err()
}

func (c *Client) Dequeue(ctx context.Context, queueName string, timeout time.Duration) (*Job, error) {
	if err := c.promoteScheduled(ctx, queueName, 100); err != nil {
		return nil, err
	}

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
	job.Normalize()
	return &job, nil
}

func (c *Client) MarkRunning(ctx context.Context, queueName string, job Job) error {
	if c.audit == nil {
		return nil
	}
	job.Normalize()
	return c.audit.RecordJobRunning(ctx, queueName, job)
}

func (c *Client) MarkCompleted(ctx context.Context, queueName string, job Job) error {
	if c.audit == nil {
		return nil
	}
	job.Normalize()
	return c.audit.RecordJobCompleted(ctx, queueName, job)
}

func (c *Client) MarkDead(ctx context.Context, queueName string, job Job, message string) error {
	if c.audit == nil {
		return nil
	}
	job.Normalize()
	return c.audit.RecordJobDead(ctx, queueName, job, message)
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

func (c *Client) scheduledKey(queueName string) string {
	return c.key(queueName + ":scheduled")
}

func (c *Client) promoteScheduled(ctx context.Context, queueName string, limit int) error {
	script := redis.NewScript(`
local items = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, ARGV[2])
local moved = 0
for _, item in ipairs(items) do
	if redis.call('ZREM', KEYS[1], item) == 1 then
		redis.call('LPUSH', KEYS[2], item)
		moved = moved + 1
	end
end
return moved
`)
	return script.Run(ctx, c.redis, []string{c.scheduledKey(queueName), c.key(queueName)}, time.Now().UTC().UnixMilli(), limit).Err()
}
