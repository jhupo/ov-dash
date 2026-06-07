CREATE TABLE IF NOT EXISTS wiki_attachments (
    id TEXT PRIMARY KEY,
    page_id TEXT REFERENCES wiki_pages(id) ON DELETE SET NULL,
    original_name TEXT NOT NULL DEFAULT '',
    storage_path TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wiki_attachments_page_created_at
    ON wiki_attachments (page_id, created_at DESC);
