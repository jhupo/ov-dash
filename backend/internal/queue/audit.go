package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/platform/redact"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	JobStatusQueued    = "queued"
	JobStatusRunning   = "running"
	JobStatusRetrying  = "retrying"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusDead      = "dead"
	JobStatusCanceled  = "canceled"
)

type JobRecord struct {
	ID                string         `json:"id"`
	QueueName         string         `json:"queue_name"`
	Type              string         `json:"type"`
	Payload           map[string]any `json:"payload"`
	IdempotencyKey    string         `json:"idempotency_key"`
	Status            string         `json:"status"`
	Attempts          int            `json:"attempts"`
	MaxAttempts       int            `json:"max_attempts"`
	LastError         string         `json:"last_error"`
	CancelRequested   bool           `json:"cancel_requested"`
	CancelRequestedAt *time.Time     `json:"cancel_requested_at,omitempty"`
	NextRunAt         *time.Time     `json:"next_run_at,omitempty"`
	StartedAt         *time.Time     `json:"started_at,omitempty"`
	CompletedAt       *time.Time     `json:"completed_at,omitempty"`
	DeadAt            *time.Time     `json:"dead_at,omitempty"`
	CanceledAt        *time.Time     `json:"canceled_at,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type JobListFilter struct {
	Types    []string
	ServerID string
	Limit    int
}

type JobLog struct {
	ID        int64          `json:"id"`
	JobID     string         `json:"job_id"`
	Stream    string         `json:"stream"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

type JobEvent struct {
	ID        int64          `json:"id"`
	JobID     string         `json:"job_id"`
	EventType string         `json:"event_type"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

type WorkerHeartbeat struct {
	ID         string    `json:"id"`
	QueueName  string    `json:"queue_name"`
	Hostname   string    `json:"hostname"`
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"started_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

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
	metadata, err := redactedMetadataJSON(map[string]any{
		"queue_name":      queueName,
		"job_type":        job.Type,
		"max_attempts":    job.MaxAttempts,
		"idempotency_key": job.IdempotencyKey,
	})
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		WITH upsert AS (
			INSERT INTO jobs_audit (
				id, queue_name, job_type, payload, status, attempts, max_attempts,
				idempotency_key, last_error, next_run_at, started_at, completed_at, dead_at, updated_at
			)
			VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, '', NULL, NULL, NULL, NULL, now())
			ON CONFLICT (id) DO UPDATE SET
				queue_name = EXCLUDED.queue_name,
				job_type = EXCLUDED.job_type,
				payload = EXCLUDED.payload,
				status = EXCLUDED.status,
				attempts = EXCLUDED.attempts,
				max_attempts = EXCLUDED.max_attempts,
				idempotency_key = EXCLUDED.idempotency_key,
				last_error = '',
				next_run_at = NULL,
				updated_at = now()
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, metadata)
		SELECT id, 'enqueued', $9::jsonb
		FROM upsert
	`, job.ID, queueName, job.Type, string(payload), JobStatusQueued, job.Attempts, job.MaxAttempts, job.IdempotencyKey, metadata)
	if isUniqueViolation(err) && job.IdempotencyKey != "" {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

func (s *PostgresAuditStore) RecordJobRunning(ctx context.Context, queueName string, job Job) error {
	job.Normalize()
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return err
	}
	metadata, err := redactedMetadataJSON(map[string]any{
		"queue_name":   queueName,
		"attempts":     job.Attempts,
		"max_attempts": job.MaxAttempts,
	})
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		WITH upsert AS (
			INSERT INTO jobs_audit (
				id, queue_name, job_type, payload, status, attempts, max_attempts,
				idempotency_key, last_error, next_run_at, started_at, updated_at
			)
			VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, '', NULL, now(), now())
			ON CONFLICT (id) DO UPDATE SET
				queue_name = EXCLUDED.queue_name,
				job_type = EXCLUDED.job_type,
				payload = EXCLUDED.payload,
				status = EXCLUDED.status,
				attempts = EXCLUDED.attempts,
				max_attempts = EXCLUDED.max_attempts,
				idempotency_key = EXCLUDED.idempotency_key,
				next_run_at = NULL,
				started_at = now(),
				updated_at = now()
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, metadata)
		SELECT id, 'started', $9::jsonb
		FROM upsert
	`, job.ID, queueName, job.Type, string(payload), JobStatusRunning, job.Attempts, job.MaxAttempts, job.IdempotencyKey, metadata)
	return err
}

func (s *PostgresAuditStore) RecordJobCompleted(ctx context.Context, queueName string, job Job) error {
	job.Normalize()
	metadata, err := redactedMetadataJSON(map[string]any{
		"queue_name": queueName,
		"attempts":   job.Attempts,
	})
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
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
		SELECT id, 'completed', $5::jsonb
		FROM updated
	`, job.ID, JobStatusCompleted, job.Attempts, job.MaxAttempts, metadata)
	return err
}

func (s *PostgresAuditStore) RecordJobFailed(ctx context.Context, queueName string, job Job, message string, nextRunAt *time.Time) error {
	job.Normalize()
	message = redactJobLogMessage(message)
	status := JobStatusFailed
	eventType := "failed"
	if nextRunAt != nil {
		status = JobStatusRetrying
		eventType = "retry_scheduled"
	}
	metadata, err := redactedMetadataJSON(map[string]any{
		"queue_name":   queueName,
		"attempts":     job.Attempts,
		"max_attempts": job.MaxAttempts,
		"next_run_at":  nextRunAt,
	})
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		WITH updated AS (
			UPDATE jobs_audit
			SET status = $2,
			    attempts = $3,
			    max_attempts = $4,
			    last_error = $5,
			    next_run_at = $6,
			    updated_at = now()
			WHERE id = $1
			  AND status NOT IN ($9, $10, $11)
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, message, metadata)
		SELECT id, $7, $5, $8::jsonb
		FROM updated
	`, job.ID, status, job.Attempts, job.MaxAttempts, message, nextRunAt, eventType, metadata, JobStatusCompleted, JobStatusDead, JobStatusCanceled)
	return err
}

func (s *PostgresAuditStore) RecordJobDead(ctx context.Context, queueName string, job Job, message string) error {
	job.Normalize()
	message = redactJobLogMessage(message)
	metadata, err := redactedMetadataJSON(map[string]any{
		"queue_name":   queueName,
		"attempts":     job.Attempts,
		"max_attempts": job.MaxAttempts,
	})
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
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
			  AND status NOT IN ($7, $8, $9)
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, message, metadata)
		SELECT id, 'dead', $5, $6::jsonb
		FROM updated
	`, job.ID, JobStatusDead, job.Attempts, job.MaxAttempts, message, metadata, JobStatusCompleted, JobStatusDead, JobStatusCanceled)
	return err
}

func (s *PostgresAuditStore) RecordJobCanceled(ctx context.Context, queueName string, job Job, message string) error {
	job.Normalize()
	message = redactJobLogMessage(message)
	metadata, err := redactedMetadataJSON(map[string]any{
		"queue_name":   queueName,
		"attempts":     job.Attempts,
		"max_attempts": job.MaxAttempts,
	})
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		WITH updated AS (
			UPDATE jobs_audit
			SET status = $2,
			    attempts = $3,
			    max_attempts = $4,
			    last_error = $5,
			    next_run_at = NULL,
			    canceled_at = now(),
			    updated_at = now()
			WHERE id = $1
			  AND status NOT IN ($7, $8, $9)
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, message, metadata)
		SELECT id, 'canceled', $5, $6::jsonb
		FROM updated
	`, job.ID, JobStatusCanceled, job.Attempts, job.MaxAttempts, message, metadata, JobStatusCompleted, JobStatusDead, JobStatusCanceled)
	return err
}

func (s *PostgresAuditStore) RecordJobRequeued(ctx context.Context, queueName string, oldJob JobRecord, newJob Job) error {
	newJob.Normalize()
	metadataJSON, err := redactedMetadataJSON(requeueMetadata(queueName, oldJob, newJob))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO job_audit_events (job_id, event_type, message, metadata)
		VALUES
			($1, 'requeued', 'job requeued', $2::jsonb),
			($3, 'requeued_from', 'job requeued from previous job', $2::jsonb)
	`, oldJob.ID, metadataJSON, newJob.ID)
	return err
}

func (s *PostgresAuditStore) ListJobs(ctx context.Context, limit int) ([]JobRecord, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, queue_name, job_type, payload, idempotency_key, status, attempts, max_attempts,
		       last_error, cancel_requested, cancel_requested_at, next_run_at, started_at,
		       completed_at, dead_at, canceled_at, created_at, updated_at
		FROM jobs_audit
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]JobRecord, 0)
	for rows.Next() {
		item, err := scanJobRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresAuditStore) ListJobsFiltered(ctx context.Context, filter JobListFilter) ([]JobRecord, error) {
	filter = normalizeJobListFilter(filter)
	rows, err := s.db.Query(ctx, `
		SELECT id::text, queue_name, job_type, payload, idempotency_key, status, attempts, max_attempts,
		       last_error, cancel_requested, cancel_requested_at, next_run_at, started_at,
		       completed_at, dead_at, canceled_at, created_at, updated_at
		FROM jobs_audit
		WHERE (COALESCE(array_length($1::text[], 1), 0) = 0 OR job_type = ANY($1::text[]))
		  AND ($2 = '' OR payload->>'server_id' = $2)
		ORDER BY created_at DESC
		LIMIT $3
	`, filter.Types, filter.ServerID, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]JobRecord, 0)
	for rows.Next() {
		item, err := scanJobRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresAuditStore) GetJob(ctx context.Context, id string) (JobRecord, error) {
	return scanJobRecord(s.db.QueryRow(ctx, `
		SELECT id::text, queue_name, job_type, payload, idempotency_key, status, attempts, max_attempts,
		       last_error, cancel_requested, cancel_requested_at, next_run_at, started_at,
		       completed_at, dead_at, canceled_at, created_at, updated_at
		FROM jobs_audit
		WHERE id = $1
	`, id))
}

func (s *PostgresAuditStore) ListJobEvents(ctx context.Context, id string) ([]JobEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, job_id::text, event_type, message, metadata, created_at
		FROM job_audit_events
		WHERE job_id = $1
		ORDER BY created_at ASC, id ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]JobEvent, 0)
	for rows.Next() {
		var item JobEvent
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.JobID, &item.EventType, &item.Message, &metadata, &item.CreatedAt); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &item.Metadata)
		}
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresAuditStore) AppendJobLog(ctx context.Context, id string, stream string, message string, metadata map[string]any) error {
	stream = strings.TrimSpace(stream)
	if stream == "" {
		stream = "system"
	}
	message = redactJobLogMessage(message)
	if message == "" {
		return nil
	}
	metadataJSON, err := redactedMetadataJSON(metadata)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO job_logs (job_id, stream, message, metadata)
		VALUES ($1, $2, $3, $4::jsonb)
	`, id, stream, message, metadataJSON)
	return err
}

func (s *PostgresAuditStore) ListJobLogs(ctx context.Context, id string) ([]JobLog, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, job_id::text, stream, message, metadata, created_at
		FROM job_logs
		WHERE job_id = $1
		ORDER BY created_at ASC, id ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]JobLog, 0)
	for rows.Next() {
		var item JobLog
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.JobID, &item.Stream, &item.Message, &metadata, &item.CreatedAt); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &item.Metadata)
		}
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresAuditStore) RequestJobCancel(ctx context.Context, id string) error {
	metadata, err := redactedMetadataJSON(nil)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		WITH updated AS (
			UPDATE jobs_audit
			SET cancel_requested = true,
			    cancel_requested_at = COALESCE(cancel_requested_at, now()),
			    updated_at = now()
			WHERE id = $1
			  AND status NOT IN ($2, $3, $4)
			RETURNING id
		)
		INSERT INTO job_audit_events (job_id, event_type, message, metadata)
		SELECT id, 'cancel_requested', '', $5::jsonb
		FROM updated
	`, id, JobStatusCompleted, JobStatusDead, JobStatusCanceled, metadata)
	return err
}

func (s *PostgresAuditStore) IsJobCancelRequested(ctx context.Context, id string) (bool, error) {
	var requested bool
	err := s.db.QueryRow(ctx, `
		SELECT cancel_requested
		FROM jobs_audit
		WHERE id = $1
	`, id).Scan(&requested)
	return requested, err
}

func (s *PostgresAuditStore) RecordWorkerHeartbeat(ctx context.Context, id string, queueName string, hostname string, pid int) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO worker_heartbeats (id, queue_name, hostname, pid, started_at, last_seen_at)
		VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (id) DO UPDATE SET
			queue_name = EXCLUDED.queue_name,
			hostname = EXCLUDED.hostname,
			pid = EXCLUDED.pid,
			last_seen_at = now()
	`, id, queueName, hostname, pid)
	return err
}

func (s *PostgresAuditStore) ListWorkerHeartbeats(ctx context.Context, maxAge time.Duration) ([]WorkerHeartbeat, error) {
	if maxAge <= 0 {
		maxAge = 30 * time.Second
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, queue_name, hostname, pid, started_at, last_seen_at
		FROM worker_heartbeats
		WHERE last_seen_at >= now() - $1::interval
		ORDER BY last_seen_at DESC
	`, maxAge.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]WorkerHeartbeat, 0)
	for rows.Next() {
		var item WorkerHeartbeat
		if err := rows.Scan(&item.ID, &item.QueueName, &item.Hostname, &item.PID, &item.StartedAt, &item.LastSeenAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type jobRecordScanner interface {
	Scan(dest ...any) error
}

func scanJobRecord(row jobRecordScanner) (JobRecord, error) {
	var item JobRecord
	var payload []byte
	err := row.Scan(
		&item.ID,
		&item.QueueName,
		&item.Type,
		&payload,
		&item.IdempotencyKey,
		&item.Status,
		&item.Attempts,
		&item.MaxAttempts,
		&item.LastError,
		&item.CancelRequested,
		&item.CancelRequestedAt,
		&item.NextRunAt,
		&item.StartedAt,
		&item.CompletedAt,
		&item.DeadAt,
		&item.CanceledAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return JobRecord{}, err
	}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &item.Payload)
	}
	if item.Payload == nil {
		item.Payload = map[string]any{}
	}
	return item, nil
}

func normalizeJobListFilter(filter JobListFilter) JobListFilter {
	filter.ServerID = strings.TrimSpace(filter.ServerID)
	seen := map[string]struct{}{}
	types := make([]string, 0, len(filter.Types))
	for _, jobType := range filter.Types {
		jobType = strings.TrimSpace(jobType)
		if jobType == "" {
			continue
		}
		if _, ok := seen[jobType]; ok {
			continue
		}
		seen[jobType] = struct{}{}
		types = append(types, jobType)
	}
	filter.Types = types
	if filter.Limit < 1 || filter.Limit > 200 {
		filter.Limit = 50
	}
	return filter
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func redactJobLogMessage(message string) string {
	return redact.Text(message)
}

func redactedMetadataJSON(metadata map[string]any) (string, error) {
	data, err := json.Marshal(redact.Metadata(metadata))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
