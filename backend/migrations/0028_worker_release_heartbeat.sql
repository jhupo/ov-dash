ALTER TABLE worker_heartbeats
    ADD COLUMN IF NOT EXISTS release_id TEXT;

DELETE FROM worker_heartbeats
WHERE release_id IS NULL OR btrim(release_id) = '';

ALTER TABLE worker_heartbeats
    ALTER COLUMN release_id SET NOT NULL;

DO $$
BEGIN
    ALTER TABLE worker_heartbeats
        ADD CONSTRAINT worker_heartbeats_release_id_not_empty
        CHECK (btrim(release_id) <> '');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END
$$;
