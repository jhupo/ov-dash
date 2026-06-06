ALTER TABLE server_connections
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS collector_installed BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS collect_status TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS collect_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_collected_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS server_metrics (
    server_id TEXT PRIMARY KEY REFERENCES server_connections(id) ON DELETE CASCADE,
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
    uptime_seconds BIGINT NOT NULL DEFAULT 0,
    architecture TEXT NOT NULL DEFAULT '',
    virtualization TEXT NOT NULL DEFAULT '',
    os_name TEXT NOT NULL DEFAULT '',
    cpu_model TEXT NOT NULL DEFAULT '',
    gpu_model TEXT NOT NULL DEFAULT '',
    raw JSONB NOT NULL DEFAULT '{}'::jsonb,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
