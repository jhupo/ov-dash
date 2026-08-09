ALTER TABLE telegram_notification_settings
    ADD COLUMN IF NOT EXISTS bot_token_secret_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS inbound_token_secret_id TEXT NOT NULL DEFAULT '';

ALTER TABLE wiki_page_resources
    ADD COLUMN IF NOT EXISTS password_secret_id TEXT NOT NULL DEFAULT '';

UPDATE wiki_page_revisions
SET resources_json = COALESCE(
    (
        SELECT jsonb_agg(
            CASE
                WHEN jsonb_typeof(resource) = 'object'
                    THEN jsonb_set(resource - 'password_secret_id', '{password}', to_jsonb(''::text), true)
                ELSE resource
            END
            ORDER BY resource_index
        )
        FROM jsonb_array_elements(resources_json) WITH ORDINALITY AS resources(resource, resource_index)
    ),
    '[]'::jsonb
)
WHERE jsonb_typeof(resources_json) = 'array'
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(resources_json) AS resources(resource)
      WHERE resource ? 'password_secret_id'
         OR COALESCE(resource->>'password', '') <> ''
  );
