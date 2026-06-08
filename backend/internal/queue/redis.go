package queue

import (
	"context"
	"encoding/json"
	"errors"
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
	RecordJobCanceled(ctx context.Context, queueName string, job Job, message string) error
	RecordJobRequeued(ctx context.Context, queueName string, oldJob JobRecord, newJob Job) error
	ListJobs(ctx context.Context, limit int) ([]JobRecord, error)
	GetJob(ctx context.Context, id string) (JobRecord, error)
	ListJobEvents(ctx context.Context, id string) ([]JobEvent, error)
	AppendJobLog(ctx context.Context, id string, stream string, message string, metadata map[string]any) error
	ListJobLogs(ctx context.Context, id string) ([]JobLog, error)
	RequestJobCancel(ctx context.Context, id string) error
	IsJobCancelRequested(ctx context.Context, id string) (bool, error)
	RecordWorkerHeartbeat(ctx context.Context, id string, queueName string, hostname string, pid int) error
	ListWorkerHeartbeats(ctx context.Context, maxAge time.Duration) ([]WorkerHeartbeat, error)
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
			return fmt.Errorf("record enqueue audit for job %s: %w", job.ID, err)
		}
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return c.recordEnqueueAfterAuditFailure(ctx, queueName, job, "redis_payload_marshal", err)
	}
	if err := c.redis.LPush(ctx, c.key(queueName), payload).Err(); err != nil {
		return c.recordEnqueueAfterAuditFailure(ctx, queueName, job, "redis_push", err)
	}
	return nil
}

func (c *Client) recordEnqueueAfterAuditFailure(ctx context.Context, queueName string, job Job, phase string, err error) error {
	wrapped := fmt.Errorf("enqueue %s failed after audit record for job %s: %w", phase, job.ID, err)
	if c.audit == nil {
		return wrapped
	}

	message := fmt.Sprintf("enqueue failed after audit record during %s: %v", phase, err)
	markErr := c.audit.RecordJobFailed(ctx, queueName, job, message, nil)
	logErr := c.audit.AppendJobLog(ctx, job.ID, "system", message, map[string]any{
		"queue_name": queueName,
		"job_id":     job.ID,
		"job_type":   job.Type,
		"phase":      phase,
		"error":      err.Error(),
	})
	if markErr != nil || logErr != nil {
		return fmt.Errorf("%w; audit update error: %v", wrapped, errors.Join(markErr, logErr))
	}
	return wrapped
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

func (c *Client) MarkCanceled(ctx context.Context, queueName string, job Job, message string) error {
	if c.audit == nil {
		return nil
	}
	job.Normalize()
	return c.audit.RecordJobCanceled(ctx, queueName, job, message)
}

func (c *Client) ListJobs(ctx context.Context, limit int) ([]JobRecord, error) {
	if c.audit == nil {
		return []JobRecord{}, nil
	}
	return c.audit.ListJobs(ctx, limit)
}

func (c *Client) GetJob(ctx context.Context, id string) (JobRecord, error) {
	if c.audit == nil {
		return JobRecord{}, nil
	}
	return c.audit.GetJob(ctx, id)
}

func (c *Client) ListJobEvents(ctx context.Context, id string) ([]JobEvent, error) {
	if c.audit == nil {
		return []JobEvent{}, nil
	}
	return c.audit.ListJobEvents(ctx, id)
}

func (c *Client) AppendJobLog(ctx context.Context, id string, stream string, message string, metadata map[string]any) error {
	if c.audit == nil {
		return nil
	}
	return c.audit.AppendJobLog(ctx, id, stream, message, metadata)
}

func (c *Client) ListJobLogs(ctx context.Context, id string) ([]JobLog, error) {
	if c.audit == nil {
		return []JobLog{}, nil
	}
	return c.audit.ListJobLogs(ctx, id)
}

func (c *Client) RequestJobCancel(ctx context.Context, id string) error {
	if c.audit == nil {
		return nil
	}
	return c.audit.RequestJobCancel(ctx, id)
}

func (c *Client) IsJobCancelRequested(ctx context.Context, id string) (bool, error) {
	if c.audit == nil {
		return false, nil
	}
	return c.audit.IsJobCancelRequested(ctx, id)
}

func (c *Client) RecordWorkerHeartbeat(ctx context.Context, id string, queueName string, hostname string, pid int) error {
	if c.audit == nil {
		return nil
	}
	return c.audit.RecordWorkerHeartbeat(ctx, id, queueName, hostname, pid)
}

func (c *Client) ListWorkerHeartbeats(ctx context.Context, maxAge time.Duration) ([]WorkerHeartbeat, error) {
	if c.audit == nil {
		return []WorkerHeartbeat{}, nil
	}
	return c.audit.ListWorkerHeartbeats(ctx, maxAge)
}

func (c *Client) RequeueJob(ctx context.Context, queueName string, id string, idempotencyKey string) (Job, error) {
	if c.audit == nil {
		return Job{}, errors.New("job audit is disabled")
	}
	record, err := c.GetJob(ctx, id)
	if err != nil {
		return Job{}, err
	}
	switch record.Status {
	case JobStatusDead, JobStatusFailed, JobStatusCanceled:
	default:
		return Job{}, ErrJobNotRequeueable
	}
	job, err := newRequeuedJob(record, idempotencyKey)
	if err != nil {
		return Job{}, err
	}
	if err := c.Enqueue(ctx, queueName, job); err != nil {
		return Job{}, err
	}
	c.recordRequeueAudit(ctx, queueName, record, job)
	return job, nil
}

func newRequeuedJob(record JobRecord, idempotencyKey string) (Job, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		job, err := NewJob(record.Type, record.Payload)
		if err != nil {
			return Job{}, err
		}
		if job.ID == record.ID {
			lastErr = fmt.Errorf("generated requeue job id matched old job id %s", record.ID)
			continue
		}
		job.IdempotencyKey = idempotencyKey
		job.MaxAttempts = record.MaxAttempts
		job.Normalize()
		return job, nil
	}
	return Job{}, fmt.Errorf("generate distinct requeue job id: %w", lastErr)
}

func (c *Client) recordRequeueAudit(ctx context.Context, queueName string, record JobRecord, job Job) {
	if c.audit == nil {
		return
	}
	metadata := requeueMetadata(queueName, record, job)
	if err := c.audit.RecordJobRequeued(ctx, queueName, record, job); err != nil {
		_ = c.audit.AppendJobLog(ctx, record.ID, "system", "record requeue event failed", map[string]any{
			"old_job_id": record.ID,
			"new_job_id": job.ID,
			"error":      err.Error(),
		})
	}
	_ = c.audit.AppendJobLog(ctx, record.ID, "system", "job requeued", metadata)
	_ = c.audit.AppendJobLog(ctx, job.ID, "system", "job requeued from previous job", metadata)
}

func requeueMetadata(queueName string, record JobRecord, job Job) map[string]any {
	return map[string]any{
		"queue_name":      queueName,
		"job_type":        job.Type,
		"old_job_id":      record.ID,
		"new_job_id":      job.ID,
		"old_status":      record.Status,
		"idempotency_key": job.IdempotencyKey,
	}
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
