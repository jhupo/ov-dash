CREATE TYPE river_job_state AS ENUM (
    'available',
    'cancelled',
    'completed',
    'discarded',
    'pending',
    'retryable',
    'running',
    'scheduled'
);

CREATE TABLE river_job (
    id BIGSERIAL PRIMARY KEY,
    state river_job_state NOT NULL DEFAULT 'available',
    attempt SMALLINT NOT NULL DEFAULT 0,
    max_attempts SMALLINT NOT NULL,
    attempted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finalized_at TIMESTAMPTZ,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    priority SMALLINT NOT NULL DEFAULT 1,
    args JSONB NOT NULL,
    attempted_by TEXT[],
    errors JSONB[],
    kind TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    queue TEXT NOT NULL DEFAULT 'default',
    tags VARCHAR(255)[] NOT NULL DEFAULT '{}',
    unique_key BYTEA,
    unique_states BIT(8),
    CONSTRAINT finalized_or_finalized_at_null CHECK (
        (finalized_at IS NULL AND state NOT IN ('cancelled', 'completed', 'discarded')) OR
        (finalized_at IS NOT NULL AND state IN ('cancelled', 'completed', 'discarded'))
    ),
    CONSTRAINT max_attempts_is_positive CHECK (max_attempts > 0),
    CONSTRAINT priority_in_range CHECK (priority BETWEEN 1 AND 4),
    CONSTRAINT queue_length CHECK (char_length(queue) > 0 AND char_length(queue) < 128),
    CONSTRAINT kind_length CHECK (char_length(kind) > 0 AND char_length(kind) < 128)
);

CREATE INDEX river_job_kind ON river_job (kind);
CREATE INDEX river_job_state_and_finalized_at_index ON river_job (state, finalized_at)
    WHERE finalized_at IS NOT NULL;
CREATE INDEX river_job_prioritized_fetching_index ON river_job (state, queue, priority, scheduled_at, id);
CREATE INDEX river_job_args_index ON river_job USING GIN (args);
CREATE INDEX river_job_metadata_index ON river_job USING GIN (metadata);

CREATE FUNCTION river_job_state_in_bitmask(bitmask BIT(8), state river_job_state)
RETURNS BOOLEAN
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT CASE state
        WHEN 'available' THEN get_bit(bitmask, 7)
        WHEN 'cancelled' THEN get_bit(bitmask, 6)
        WHEN 'completed' THEN get_bit(bitmask, 5)
        WHEN 'discarded' THEN get_bit(bitmask, 4)
        WHEN 'pending' THEN get_bit(bitmask, 3)
        WHEN 'retryable' THEN get_bit(bitmask, 2)
        WHEN 'running' THEN get_bit(bitmask, 1)
        WHEN 'scheduled' THEN get_bit(bitmask, 0)
        ELSE 0
    END = 1;
$$;

CREATE UNIQUE INDEX river_job_unique_idx ON river_job (unique_key)
    WHERE unique_key IS NOT NULL
      AND unique_states IS NOT NULL
      AND river_job_state_in_bitmask(unique_states, state);

CREATE UNLOGGED TABLE river_leader (
    elected_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    leader_id TEXT NOT NULL,
    name TEXT PRIMARY KEY DEFAULT 'default',
    CONSTRAINT name_length CHECK (name = 'default'),
    CONSTRAINT leader_id_length CHECK (char_length(leader_id) > 0 AND char_length(leader_id) < 128)
);

CREATE TABLE river_queue (
    name TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    paused_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNLOGGED TABLE river_client (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    paused_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT river_client_id_length CHECK (char_length(id) > 0 AND char_length(id) < 128)
);

CREATE UNLOGGED TABLE river_client_queue (
    river_client_id TEXT NOT NULL REFERENCES river_client (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    max_workers BIGINT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    num_jobs_completed BIGINT NOT NULL DEFAULT 0,
    num_jobs_running BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (river_client_id, name),
    CONSTRAINT river_client_queue_name_length CHECK (char_length(name) > 0 AND char_length(name) < 128),
    CONSTRAINT num_jobs_completed_zero_or_positive CHECK (num_jobs_completed >= 0),
    CONSTRAINT num_jobs_running_zero_or_positive CHECK (num_jobs_running >= 0)
);

CREATE TABLE river_migration (
    line TEXT NOT NULL,
    version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT river_migration_line_length CHECK (char_length(line) > 0 AND char_length(line) < 128),
    CONSTRAINT river_migration_version_gte_1 CHECK (version >= 1),
    PRIMARY KEY (line, version)
);

INSERT INTO river_migration (line, version)
SELECT 'main', version
FROM generate_series(1, 6) AS version;

ALTER TABLE jobs_audit
    ADD COLUMN river_job_id BIGINT UNIQUE REFERENCES river_job(id) ON DELETE SET NULL;

UPDATE jobs_audit
SET status = 'failed',
    last_error = 'legacy Redis queue removed during River migration',
    next_run_at = NULL,
    updated_at = now()
WHERE status IN ('queued', 'running', 'retrying');
