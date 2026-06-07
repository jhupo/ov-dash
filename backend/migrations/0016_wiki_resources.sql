CREATE TABLE IF NOT EXISTS wiki_page_resources (
    id TEXT PRIMARY KEY,
    page_id TEXT NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    host TEXT NOT NULL DEFAULT '',
    port TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    password TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_resources_page_sort
    ON wiki_page_resources (page_id, sort_order ASC, title ASC);

INSERT INTO wiki_page_resources (
    id,
    page_id,
    resource_type,
    title,
    host,
    port,
    url,
    username,
    note,
    sort_order
)
SELECT
    id || '-machine',
    id,
    'machine',
    CASE WHEN title = '' THEN '机器信息' ELSE title END,
    machine_host,
    machine_port,
    '',
    machine_username,
    '由旧版机器字段迁移生成。',
    1
FROM wiki_pages
WHERE machine_host <> ''
ON CONFLICT (id) DO NOTHING;

INSERT INTO wiki_page_resources (
    id,
    page_id,
    resource_type,
    title,
    host,
    port,
    url,
    username,
    note,
    sort_order
)
SELECT
    id || '-link',
    id,
    'link',
    CASE WHEN link_label = '' THEN '关联链接' ELSE link_label END,
    '',
    '',
    link_url,
    '',
    '由旧版链接字段迁移生成。',
    2
FROM wiki_pages
WHERE link_url <> ''
ON CONFLICT (id) DO NOTHING;

ALTER TABLE wiki_pages
    DROP COLUMN IF EXISTS link_label,
    DROP COLUMN IF EXISTS link_url,
    DROP COLUMN IF EXISTS machine_host,
    DROP COLUMN IF EXISTS machine_port,
    DROP COLUMN IF EXISTS machine_username;

ALTER TABLE wiki_page_revisions
    DROP COLUMN IF EXISTS link_label,
    DROP COLUMN IF EXISTS link_url,
    DROP COLUMN IF EXISTS machine_host,
    DROP COLUMN IF EXISTS machine_port,
    DROP COLUMN IF EXISTS machine_username;
