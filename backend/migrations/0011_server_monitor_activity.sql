CREATE TABLE IF NOT EXISTS server_monitor_activity (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    active_until timestamptz NOT NULL DEFAULT 'epoch'::timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO server_monitor_activity (id, active_until)
VALUES (true, 'epoch'::timestamptz)
ON CONFLICT (id) DO NOTHING;
