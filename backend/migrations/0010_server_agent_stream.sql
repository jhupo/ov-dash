ALTER TABLE server_connections
    ADD COLUMN IF NOT EXISTS agent_port INTEGER NOT NULL DEFAULT 19087,
    ADD COLUMN IF NOT EXISTS agent_last_seen_at TIMESTAMPTZ;

UPDATE server_connections
SET next_collect_at = now();
