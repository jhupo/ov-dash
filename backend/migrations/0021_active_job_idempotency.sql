DROP INDEX IF EXISTS idx_jobs_audit_queue_idempotency_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_audit_queue_idempotency_key
    ON jobs_audit (queue_name, idempotency_key)
    WHERE idempotency_key <> ''
      AND status IN ('queued', 'running', 'retrying', 'failed');
