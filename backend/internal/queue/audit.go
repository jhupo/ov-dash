package queue

import (
	"context"
	"encoding/json"
	"time"

	"ov-dash/backend/internal/db"
)

const (
	JobStatusQueued    = "queued"
	JobStatusRunning   = "running"
	JobStatusRetrying  = "retrying"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusDead      = "dead"
)

type PostgresAuditStore struct {
	db *db.Pool
}

func NewPostgresAuditStore(db *db.Pool) *PostgresAuditStore {
	return &PostgresAuditStore{db: db}
}

func (s *PostgresAuditStore) RecordJobEnqueued(ctx context.Context, queueName string, job Job) error {
	job.Normalize()
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		WITH upsert AS (
			INSERT INTO jobs_audit (
				id, queue_name, job_type, payload, status, attempts, max_attempts,
				last_error, next_run_at, started_at, completed_at, dead_at, updated_at
			)
			VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, '', NULL, NULL, NULL, NULL, now())
			ON CONFLICT (id) DO UPDATE SET
				queue_name = EXCLUDED.queue_name,
				job_type = EXCLUDED.job_type,
				payload = EXCLUDED.payload,
				status = EXCLUDED.status,
				attempts = EXCLUDED.attempts,
				max_attempts = EXCLUDED.max_attempts,
				last_error = '',
				next_run_at = NULL,
				updated_at = now()
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, metadata)
		SELECT id, 'enqueued', jsonb_build_object('queue_name', $2, 'job_type', $3, 'max_attempts', $7)
		FROM upsert
	`, job.ID, queueName, job.Type, string(payload), JobStatusQueued, job.Attempts, job.MaxAttempts)
	return err
}

func (s *PostgresAuditStore) RecordJobRunning(ctx context.Context, queueName string, job Job) error {
	job.Normalize()
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		WITH upsert AS (
			INSERT INTO jobs_audit (
				id, queue_name, job_type, payload, status, attempts, max_attempts,
				last_error, next_run_at, started_at, updated_at
			)
			VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, '', NULL, now(), now())
			ON CONFLICT (id) DO UPDATE SET
				queue_name = EXCLUDED.queue_name,
				job_type = EXCLUDED.job_type,
				payload = EXCLUDED.payload,
				status = EXCLUDED.status,
				attempts = EXCLUDED.attempts,
				max_attempts = EXCLUDED.max_attempts,
				next_run_at = NULL,
				started_at = now(),
				updated_at = now()
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, metadata)
		SELECT id, 'started', jsonb_build_object('queue_name', $2, 'attempts', $6, 'max_attempts', $7)
		FROM upsert
	`, job.ID, queueName, job.Type, string(payload), JobStatusRunning, job.Attempts, job.MaxAttempts)
	return err
}

func (s *PostgresAuditStore) RecordJobCompleted(ctx context.Context, queueName string, job Job) error {
	job.Normalize()
	_, err := s.db.Exec(ctx, `
		WITH updated AS (
			UPDATE jobs_audit
			SET status = $2,
			    attempts = $3,
			    max_attempts = $4,
			    last_error = '',
			    next_run_at = NULL,
			    completed_at = now(),
			    updated_at = now()
			WHERE id = $1
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, metadata)
		SELECT id, 'completed', jsonb_build_object('queue_name', $5, 'attempts', $3)
		FROM updated
	`, job.ID, JobStatusCompleted, job.Attempts, job.MaxAttempts, queueName)
	return err
}

func (s *PostgresAuditStore) RecordJobFailed(ctx context.Context, queueName string, job Job, message string, nextRunAt *time.Time) error {
	job.Normalize()
	status := JobStatusFailed
	eventType := "failed"
	if nextRunAt != nil {
		status = JobStatusRetrying
		eventType = "retry_scheduled"
	}

	_, err := s.db.Exec(ctx, `
		WITH updated AS (
			UPDATE jobs_audit
			SET status = $2,
			    attempts = $3,
			    max_attempts = $4,
			    last_error = $5,
			    next_run_at = $6,
			    updated_at = now()
			WHERE id = $1
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, message, metadata)
		SELECT id, $7, $5, jsonb_build_object('queue_name', $8, 'attempts', $3, 'max_attempts', $4, 'next_run_at', $6::timestamptz)
		FROM updated
	`, job.ID, status, job.Attempts, job.MaxAttempts, message, nextRunAt, eventType, queueName)
	return err
}

func (s *PostgresAuditStore) RecordJobDead(ctx context.Context, queueName string, job Job, message string) error {
	job.Normalize()
	_, err := s.db.Exec(ctx, `
		WITH updated AS (
			UPDATE jobs_audit
			SET status = $2,
			    attempts = $3,
			    max_attempts = $4,
			    last_error = $5,
			    next_run_at = NULL,
			    dead_at = now(),
			    updated_at = now()
			WHERE id = $1
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, message, metadata)
		SELECT id, 'dead', $5, jsonb_build_object('queue_name', $6, 'attempts', $3, 'max_attempts', $4)
		FROM updated
	`, job.ID, JobStatusDead, job.Attempts, job.MaxAttempts, message, queueName)
	return err
}
