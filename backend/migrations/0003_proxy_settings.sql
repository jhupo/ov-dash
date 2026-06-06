CREATE TABLE IF NOT EXISTS proxy_settings (
    id TEXT PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT false,
    scheme TEXT NOT NULL DEFAULT 'socks5',
    host TEXT NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 1080,
    username TEXT NOT NULL DEFAULT '',
    password TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO proxy_settings (id)
VALUES ('default')
ON CONFLICT (id) DO NOTHING;
