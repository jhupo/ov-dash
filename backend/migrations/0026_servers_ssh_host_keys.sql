CREATE TABLE server_ssh_host_keys (
    server_id TEXT PRIMARY KEY REFERENCES server_connections(id) ON DELETE CASCADE,
    algorithm TEXT NOT NULL,
    public_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_verified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (algorithm <> ''),
    CHECK (public_key <> ''),
    CHECK (fingerprint <> '')
);
