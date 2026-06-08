CREATE TABLE IF NOT EXISTS secrets (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    name TEXT NOT NULL,
    nonce TEXT NOT NULL,
    ciphertext TEXT NOT NULL,
    key_id TEXT NOT NULL DEFAULT 'local',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope, name)
);

CREATE INDEX IF NOT EXISTS idx_secrets_scope_name
    ON secrets (scope, name);

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    actor_id TEXT NOT NULL DEFAULT '',
    actor_email TEXT NOT NULL DEFAULT '',
    actor_role TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    resource TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    result TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_resource_created_at
    ON audit_logs (resource, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_created_at
    ON audit_logs (actor_id, created_at DESC);

ALTER TABLE proxy_settings
    ADD COLUMN IF NOT EXISTS password_secret_id TEXT NOT NULL DEFAULT '';

ALTER TABLE server_connections
    ADD COLUMN IF NOT EXISTS password_secret_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS private_key_secret_id TEXT NOT NULL DEFAULT '';

ALTER TABLE jobs_audit
    ADD COLUMN IF NOT EXISTS cancel_requested BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS canceled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_jobs_audit_cancel_requested
    ON jobs_audit (cancel_requested, updated_at DESC)
    WHERE cancel_requested = true;
