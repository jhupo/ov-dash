ALTER TABLE telegram_notification_settings
    ADD COLUMN IF NOT EXISTS group_enabled BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS group_chat_id TEXT NOT NULL DEFAULT '';
