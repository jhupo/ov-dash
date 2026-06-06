CREATE TABLE IF NOT EXISTS telegram_notification_settings (
    id TEXT PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT false,
    bot_token TEXT NOT NULL DEFAULT '',
    inbound_token TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO telegram_notification_settings (id)
VALUES ('default')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS user_telegram_settings (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT false,
    chat_id TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
