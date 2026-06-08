ALTER TABLE server_connections
    ADD COLUMN IF NOT EXISTS collect_failure_count INTEGER NOT NULL DEFAULT 0;

UPDATE server_connections
SET collect_failure_count = 0
WHERE collect_failure_count < 0;
