ALTER TABLE jobs_audit
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_audit_queue_idempotency_key
    ON jobs_audit (queue_name, idempotency_key)
    WHERE idempotency_key <> '';

CREATE TABLE IF NOT EXISTS job_logs (
    id BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs_audit(id) ON DELETE CASCADE,
    stream TEXT NOT NULL DEFAULT 'system',
    message TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_job_logs_job_created_at
    ON job_logs (job_id, created_at ASC, id ASC);

CREATE TABLE IF NOT EXISTS worker_heartbeats (
    id TEXT PRIMARY KEY,
    queue_name TEXT NOT NULL,
    hostname TEXT NOT NULL DEFAULT '',
    pid INTEGER NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_worker_heartbeats_last_seen_at
    ON worker_heartbeats (last_seen_at DESC);
