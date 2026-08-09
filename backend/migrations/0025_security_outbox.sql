CREATE TABLE IF NOT EXISTS role_capabilities (
    role TEXT NOT NULL,
    capability TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (role, capability),
    CHECK (role <> ''),
    CHECK (capability <> '')
);

INSERT INTO role_capabilities (role, capability)
VALUES
    ('viewer', 'dashboard:read'),
    ('viewer', 'platform:read'),
    ('viewer', 'tasks:read'),
    ('viewer', 'apps:read'),
    ('viewer', 'chats:read'),
    ('viewer', 'users:read'),
    ('viewer', 'wiki:read'),
    ('viewer', 'settings:read'),
    ('viewer', 'proxy:read'),
    ('viewer', 'notifications:read'),
    ('viewer', 'updates:read'),
    ('viewer', 'jobs:read'),
    ('viewer', 'servers:read'),
    ('operator', 'dashboard:read'),
    ('operator', 'platform:read'),
    ('operator', 'tasks:read'),
    ('operator', 'tasks:write'),
    ('operator', 'apps:read'),
    ('operator', 'chats:read'),
    ('operator', 'users:read'),
    ('operator', 'wiki:read'),
    ('operator', 'wiki:write'),
    ('operator', 'settings:read'),
    ('operator', 'settings:write'),
    ('operator', 'proxy:read'),
    ('operator', 'proxy:write'),
    ('operator', 'notifications:read'),
    ('operator', 'notifications:write'),
    ('operator', 'updates:read'),
    ('operator', 'jobs:read'),
    ('operator', 'jobs:create'),
    ('operator', 'jobs:manage'),
    ('operator', 'servers:read'),
    ('operator', 'servers:write'),
    ('operator', 'servers:ssh')
ON CONFLICT (role, capability) DO NOTHING;

CREATE TABLE IF NOT EXISTS event_outbox (
    id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 10,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (event_type <> ''),
    CHECK (attempts >= 0),
    CHECK (max_attempts > 0)
);

CREATE INDEX IF NOT EXISTS idx_event_outbox_claim
    ON event_outbox (available_at, created_at, id)
    WHERE delivered_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_event_outbox_lease
    ON event_outbox (lease_expires_at)
    WHERE delivered_at IS NULL AND lease_expires_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_event_outbox_delivered
    ON event_outbox (delivered_at)
    WHERE delivered_at IS NOT NULL;
