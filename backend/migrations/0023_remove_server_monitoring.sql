DROP TABLE IF EXISTS server_monitor_activity;
DROP TABLE IF EXISTS server_metric_samples;
DROP TABLE IF EXISTS server_metrics;

DROP INDEX IF EXISTS idx_server_connections_next_collect_at;

ALTER TABLE server_connections
    DROP COLUMN IF EXISTS collector_installed,
    DROP COLUMN IF EXISTS collect_status,
    DROP COLUMN IF EXISTS collect_error,
    DROP COLUMN IF EXISTS collect_failure_count,
    DROP COLUMN IF EXISTS collect_interval_seconds,
    DROP COLUMN IF EXISTS next_collect_at,
    DROP COLUMN IF EXISTS last_collected_at,
    DROP COLUMN IF EXISTS agent_port,
    DROP COLUMN IF EXISTS agent_last_seen_at;
