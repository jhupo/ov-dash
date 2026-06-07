ALTER TABLE wiki_page_revisions
    ADD COLUMN IF NOT EXISTS resources_json JSONB NOT NULL DEFAULT '[]'::jsonb;

WITH resource_snapshots AS (
    SELECT
        page_id,
        jsonb_agg(
            jsonb_build_object(
                'id', id,
                'page_id', page_id,
                'resource_type', resource_type,
                'title', title,
                'host', host,
                'port', port,
                'url', url,
                'username', username,
                'password', password,
                'note', note,
                'sort_order', sort_order,
                'created_at', created_at,
                'updated_at', updated_at
            )
            ORDER BY sort_order ASC, title ASC
        ) AS resources_json
    FROM wiki_page_resources
    GROUP BY page_id
)
UPDATE wiki_page_revisions AS revision
SET resources_json = resource_snapshots.resources_json
FROM resource_snapshots
WHERE revision.page_id = resource_snapshots.page_id
  AND revision.resources_json = '[]'::jsonb;
