package queue

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"ov-dash/backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

const (
	EnvelopeKind    = "ov_dash.job"
	EnvelopeVersion = 1
)

type Envelope struct {
	Version int `json:"version"`
	Job     Job `json:"job"`
}

func (Envelope) Kind() string { return EnvelopeKind }

func (e *Envelope) UnmarshalJSON(data []byte) error {
	type envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode((*envelope)(e)); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("envelope contains multiple JSON values")
		}
		return err
	}
	return e.Validate()
}

func (e Envelope) Validate() error {
	if e.Version != EnvelopeVersion {
		return fmt.Errorf("unsupported envelope version: %d", e.Version)
	}
	if err := validateJobID(e.Job.ID); err != nil {
		return err
	}
	if strings.TrimSpace(e.Job.Type) == "" || e.Job.Type != strings.TrimSpace(e.Job.Type) {
		return errors.New("envelope job type is invalid")
	}
	if e.Job.Payload == nil {
		return errors.New("envelope job payload is required")
	}
	if e.Job.MaxAttempts < 1 {
		return errors.New("envelope max attempts must be positive")
	}
	return nil
}

type Dispatcher interface {
	Dispatch(context.Context, Job) error
	Timeout(Job) time.Duration
}

type WorkerOptions struct {
	QueueName       string
	Concurrency     int
	JobTimeout      time.Duration
	RescueAfter     time.Duration
	ShutdownTimeout time.Duration
	InstanceID      string
	Started         func()
}

type workerClient interface {
	Start(context.Context) error
	Stop(context.Context) error
	StopAndCancel(context.Context) error
}

type riverTransactionClient interface {
	InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
	JobCancelTx(context.Context, pgx.Tx, int64) (*rivertype.JobRow, error)
}

type Client struct {
	db    *db.Pool
	begin func(context.Context) (pgx.Tx, error)
	river riverTransactionClient
	audit *PostgresAuditStore
}

func Open(pg *db.Pool) (*Client, error) {
	if pg == nil {
		return nil, errors.New("queue postgres pool is required")
	}
	riverClient, err := river.NewClient[pgx.Tx](riverpgxv5.New(pg), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("create River insert client: %w", err)
	}
	return &Client{
		db:    pg,
		begin: pg.Begin,
		river: riverClient,
		audit: NewPostgresAuditStore(pg),
	}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.db.Ping(pingCtx)
}

func (c *Client) Enqueue(ctx context.Context, queueName string, job Job) error {
	job.Normalize()
	envelope := Envelope{Version: EnvelopeVersion, Job: job}
	if err := envelope.Validate(); err != nil {
		return err
	}

	tx, err := c.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if err := c.insertTx(ctx, tx, queueName, job); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit River enqueue: %w", err)
	}
	return nil
}

func (c *Client) insertTx(ctx context.Context, tx pgx.Tx, queueName string, job Job) error {
	if err := c.audit.RecordJobEnqueuedTx(ctx, tx, queueName, job); err != nil {
		return fmt.Errorf("record enqueue audit for job %s: %w", job.ID, err)
	}

	opts := &river.InsertOpts{
		MaxAttempts: job.MaxAttempts,
		Queue:       queueName,
	}
	if job.ScheduledAt != nil {
		opts.ScheduledAt = *job.ScheduledAt
	}
	result, err := c.river.InsertTx(ctx, tx, Envelope{Version: EnvelopeVersion, Job: job}, opts)
	if err != nil {
		return fmt.Errorf("insert River job %s: %w", job.ID, err)
	}
	if err := c.audit.LinkRiverJobTx(ctx, tx, job.ID, result.Job.ID); err != nil {
		return fmt.Errorf("link River job %s: %w", job.ID, err)
	}
	if err := c.audit.appendJobOutbox(ctx, tx, "job.enqueued", jobLifecyclePayload(queueName, job, map[string]any{
		"idempotency_key": job.IdempotencyKey,
	})); err != nil {
		return fmt.Errorf("append enqueue outbox for job %s: %w", job.ID, err)
	}
	return nil
}

func (c *Client) RunWorker(ctx context.Context, opts WorkerOptions, dispatcher Dispatcher) error {
	if dispatcher == nil {
		return errors.New("queue dispatcher is required")
	}
	if strings.TrimSpace(opts.QueueName) == "" {
		return errors.New("worker queue name is required")
	}
	if opts.Concurrency < 1 {
		return errors.New("worker concurrency must be greater than zero")
	}
	if opts.JobTimeout <= 0 {
		return errors.New("worker job timeout must be greater than zero")
	}
	if opts.RescueAfter <= opts.JobTimeout {
		return errors.New("worker rescue interval must be greater than job timeout")
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 15 * time.Second
	}

	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &envelopeWorker{dispatcher: dispatcher}); err != nil {
		return fmt.Errorf("register River envelope worker: %w", err)
	}
	workerClient, err := river.NewClient[pgx.Tx](riverpgxv5.New(c.db), &river.Config{
		ID:                   opts.InstanceID,
		JobTimeout:           opts.JobTimeout,
		MaxAttempts:          DefaultMaxAttempts,
		RescueStuckJobsAfter: opts.RescueAfter,
		Queues: map[string]river.QueueConfig{
			opts.QueueName: {MaxWorkers: opts.Concurrency},
		},
		Workers: workers,
	})
	if err != nil {
		return fmt.Errorf("create River worker client: %w", err)
	}
	return runWorkerClient(ctx, opts, workerClient)
}

func runWorkerClient(ctx context.Context, opts WorkerOptions, client workerClient) error {
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("start River worker client: %w", err)
	}
	if opts.Started != nil {
		opts.Started()
	}

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
	defer cancel()
	if err := client.Stop(shutdownCtx); err != nil {
		cancelCtx, cancelWorkers := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
		defer cancelWorkers()
		if cancelErr := client.StopAndCancel(cancelCtx); cancelErr != nil {
			return errors.Join(err, cancelErr)
		}
	}
	return nil
}

type envelopeWorker struct {
	river.WorkerDefaults[Envelope]
	dispatcher Dispatcher
}

func (w *envelopeWorker) Work(ctx context.Context, riverJob *river.Job[Envelope]) error {
	if err := riverJob.Args.Validate(); err != nil {
		return err
	}
	job := riverJob.Args.Job
	job.Attempts = riverJob.Attempt
	job.MaxAttempts = riverJob.MaxAttempts
	return w.dispatcher.Dispatch(ctx, job)
}

func (w *envelopeWorker) Timeout(riverJob *river.Job[Envelope]) time.Duration {
	job := riverJob.Args.Job
	job.Attempts = riverJob.Attempt
	job.MaxAttempts = riverJob.MaxAttempts
	return w.dispatcher.Timeout(job)
}

func (w *envelopeWorker) NextRetry(riverJob *river.Job[Envelope]) time.Time {
	base := time.Now().UTC()
	if riverJob.AttemptedAt != nil {
		base = riverJob.AttemptedAt.UTC()
	}
	return base.Add(RetryBackoff(riverJob.Attempt))
}

func RetryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 5 * time.Second
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= time.Minute {
			return time.Minute
		}
	}
	return delay
}

func (c *Client) MarkRunning(ctx context.Context, queueName string, job Job) error {
	return c.audit.RecordJobRunning(ctx, queueName, job)
}

func (c *Client) MarkCompleted(ctx context.Context, queueName string, job Job) error {
	return c.audit.RecordJobCompleted(ctx, queueName, job)
}

func (c *Client) MarkFailed(ctx context.Context, queueName string, job Job, message string, nextRunAt *time.Time) error {
	return c.audit.RecordJobFailed(ctx, queueName, job, message, nextRunAt)
}

func (c *Client) MarkDead(ctx context.Context, queueName string, job Job, message string) error {
	return c.audit.RecordJobDead(ctx, queueName, job, message)
}

func (c *Client) MarkCanceled(ctx context.Context, queueName string, job Job, message string) error {
	return c.audit.RecordJobCanceled(ctx, queueName, job, message)
}

func (c *Client) ListJobs(ctx context.Context, limit int) ([]JobRecord, error) {
	return c.audit.ListJobs(ctx, limit)
}

func (c *Client) ListJobsFiltered(ctx context.Context, filter JobListFilter) ([]JobRecord, error) {
	return c.audit.ListJobsFiltered(ctx, filter)
}

func (c *Client) GetJob(ctx context.Context, id string) (JobRecord, error) {
	return c.audit.GetJob(ctx, id)
}

func (c *Client) ListJobEvents(ctx context.Context, id string) ([]JobEvent, error) {
	return c.audit.ListJobEvents(ctx, id)
}

func (c *Client) AppendJobLog(ctx context.Context, id string, stream string, message string, metadata map[string]any) error {
	return c.audit.AppendJobLog(ctx, id, stream, message, metadata)
}

func (c *Client) ListJobLogs(ctx context.Context, id string) ([]JobLog, error) {
	return c.audit.ListJobLogs(ctx, id)
}

func (c *Client) IsJobCancelRequested(ctx context.Context, id string) (bool, error) {
	return c.audit.IsJobCancelRequested(ctx, id)
}

func (c *Client) RequestJobCancel(ctx context.Context, id string) error {
	tx, err := c.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))

	var riverJobID int64
	var jobType string
	var queueName string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(river_job_id, 0), job_type, queue_name
		FROM jobs_audit
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&riverJobID, &jobType, &queueName); err != nil {
		return err
	}
	if riverJobID == 0 {
		return errors.New("job is not linked to River")
	}
	riverJob, err := c.river.JobCancelTx(ctx, tx, riverJobID)
	if err != nil {
		return fmt.Errorf("cancel River job: %w", err)
	}

	immediatelyCanceled := riverJob.State == rivertype.JobStateCancelled
	statusSQL := ""
	if immediatelyCanceled {
		statusSQL = ", status = 'canceled', canceled_at = COALESCE(canceled_at, now())"
	}
	metadata, err := redactedMetadataJSON(map[string]any{"river_job_id": riverJobID})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		WITH updated AS (
			UPDATE jobs_audit
			SET cancel_requested = true,
			    cancel_requested_at = COALESCE(cancel_requested_at, now()),
			    updated_at = now()
			    `+statusSQL+`
			WHERE id = $1
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, metadata)
		SELECT id, 'cancel_requested', $2::jsonb FROM updated
	`, id, metadata); err != nil {
		return err
	}
	payload := map[string]any{
		"job_id":       id,
		"job_type":     jobType,
		"queue_name":   queueName,
		"river_job_id": riverJobID,
	}
	if err := c.audit.appendJobOutbox(ctx, tx, "job.cancel_requested", payload); err != nil {
		return err
	}
	if immediatelyCanceled {
		if _, err := tx.Exec(ctx, `
			INSERT INTO job_audit_events (job_id, event_type, metadata)
			VALUES ($1, 'canceled', $2::jsonb)
		`, id, metadata); err != nil {
			return err
		}
		if err := c.audit.appendJobOutbox(ctx, tx, "job.canceled", payload); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (c *Client) RequeueJob(ctx context.Context, queueName string, id string, idempotencyKey string) (Job, error) {
	tx, err := c.begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))

	record, err := scanJobRecord(tx.QueryRow(ctx, `
		SELECT id::text, queue_name, job_type, payload, idempotency_key, status, attempts, max_attempts,
		       last_error, cancel_requested, cancel_requested_at, next_run_at, started_at,
		       completed_at, dead_at, canceled_at, created_at, updated_at, COALESCE(river_job_id, 0)
		FROM jobs_audit
		WHERE id = $1
		FOR UPDATE
	`, id))
	if err != nil {
		return Job{}, err
	}
	switch record.Status {
	case JobStatusDead, JobStatusFailed, JobStatusCanceled:
	default:
		return Job{}, ErrJobNotRequeueable
	}
	if record.Status == JobStatusFailed {
		if record.RiverJobID == 0 {
			return Job{}, errors.New("failed job is not linked to River")
		}
		if _, err := c.river.JobCancelTx(ctx, tx, record.RiverJobID); err != nil {
			return Job{}, fmt.Errorf("cancel failed River job before requeue: %w", err)
		}
	}
	job, err := newRequeuedJob(record, idempotencyKey)
	if err != nil {
		return Job{}, err
	}
	if err := c.insertTx(ctx, tx, queueName, job); err != nil {
		return Job{}, err
	}
	metadata, err := redactedMetadataJSON(requeueMetadata(queueName, record, job))
	if err != nil {
		return Job{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_audit_events (job_id, event_type, message, metadata)
		VALUES
			($1, 'requeued', 'job requeued', $3::jsonb),
			($2, 'requeued_from', 'job requeued from previous job', $3::jsonb)
	`, record.ID, job.ID, metadata); err != nil {
		return Job{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_logs (job_id, stream, message, metadata)
		VALUES
			($1, 'system', 'job requeued', $3::jsonb),
			($2, 'system', 'job requeued from previous job', $3::jsonb)
	`, record.ID, job.ID, metadata); err != nil {
		return Job{}, err
	}
	eventPayload := requeueMetadata(queueName, record, job)
	eventPayload["job_id"] = record.ID
	if err := c.audit.appendJobOutbox(ctx, tx, "job.requeued", eventPayload); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (c *Client) RecordWorkerHeartbeat(ctx context.Context, id string, queueName string, releaseID string, hostname string, pid int) error {
	return c.audit.RecordWorkerHeartbeat(ctx, id, queueName, releaseID, hostname, pid)
}

func (c *Client) ListWorkerHeartbeats(ctx context.Context, maxAge time.Duration) ([]WorkerHeartbeat, error) {
	return c.audit.ListWorkerHeartbeats(ctx, maxAge)
}

func NormalizeReleaseID(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "ov-dash-")
	version = strings.TrimPrefix(version, "v")
	if version == "" {
		return ""
	}
	return "ov-dash-" + version
}

func newRequeuedJob(record JobRecord, idempotencyKey string) (Job, error) {
	job, err := NewJob(record.Type, record.Payload)
	if err != nil {
		return Job{}, err
	}
	job.IdempotencyKey = idempotencyKey
	job.MaxAttempts = record.MaxAttempts
	job.Normalize()
	return job, nil
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

func validateJobID(id string) error {
	if len(id) != 32 || strings.ToLower(id) != id {
		return errors.New("envelope job id must be 32 lowercase hexadecimal characters")
	}
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 16 {
		return errors.New("envelope job id must be 32 lowercase hexadecimal characters")
	}
	return nil
}
