ALTER TABLE jobs_audit
    ADD COLUMN IF NOT EXISTS queue_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_attempts INTEGER NOT NULL DEFAULT 3,
    ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS dead_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS next_run_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_jobs_audit_queue_status_created_at
    ON jobs_audit (queue_name, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_jobs_audit_next_run_at
    ON jobs_audit (next_run_at)
    WHERE next_run_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS job_audit_events (
    id BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs_audit(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_job_audit_events_job_created_at
    ON job_audit_events (job_id, created_at DESC);
