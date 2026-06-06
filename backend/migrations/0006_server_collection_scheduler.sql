ALTER TABLE server_connections
    ADD COLUMN IF NOT EXISTS collect_interval_seconds INTEGER NOT NULL DEFAULT 300,
    ADD COLUMN IF NOT EXISTS next_collect_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE server_metrics
    ADD COLUMN IF NOT EXISTS region TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tcp_connections BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS udp_connections BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS process_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cpu_cores INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS latency_ms DOUBLE PRECISION NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_server_connections_next_collect_at
    ON server_connections (next_collect_at, collect_status);

CREATE TABLE IF NOT EXISTS server_metric_samples (
    id BIGSERIAL PRIMARY KEY,
    server_id TEXT NOT NULL REFERENCES server_connections(id) ON DELETE CASCADE,
    cpu_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_used_bytes BIGINT NOT NULL DEFAULT 0,
    memory_total_bytes BIGINT NOT NULL DEFAULT 0,
    swap_used_bytes BIGINT NOT NULL DEFAULT 0,
    swap_total_bytes BIGINT NOT NULL DEFAULT 0,
    disk_used_bytes BIGINT NOT NULL DEFAULT 0,
    disk_total_bytes BIGINT NOT NULL DEFAULT 0,
    network_rx_bytes BIGINT NOT NULL DEFAULT 0,
    network_tx_bytes BIGINT NOT NULL DEFAULT 0,
    network_rx_rate_bps DOUBLE PRECISION NOT NULL DEFAULT 0,
    network_tx_rate_bps DOUBLE PRECISION NOT NULL DEFAULT 0,
    load1 DOUBLE PRECISION NOT NULL DEFAULT 0,
    load5 DOUBLE PRECISION NOT NULL DEFAULT 0,
    load15 DOUBLE PRECISION NOT NULL DEFAULT 0,
    tcp_connections BIGINT NOT NULL DEFAULT 0,
    udp_connections BIGINT NOT NULL DEFAULT 0,
    process_count BIGINT NOT NULL DEFAULT 0,
    cpu_cores INTEGER NOT NULL DEFAULT 0,
    latency_ms DOUBLE PRECISION NOT NULL DEFAULT 0,
    uptime_seconds BIGINT NOT NULL DEFAULT 0,
    architecture TEXT NOT NULL DEFAULT '',
    virtualization TEXT NOT NULL DEFAULT '',
    os_name TEXT NOT NULL DEFAULT '',
    cpu_model TEXT NOT NULL DEFAULT '',
    gpu_model TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL DEFAULT '',
    raw JSONB NOT NULL DEFAULT '{}'::jsonb,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_server_metric_samples_server_collected_at
    ON server_metric_samples (server_id, collected_at DESC);
