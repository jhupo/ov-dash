CREATE TABLE IF NOT EXISTS wiki_pages (
    id TEXT PRIMARY KEY,
    parent_id TEXT REFERENCES wiki_pages(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    page_type TEXT NOT NULL DEFAULT 'document',
    category TEXT NOT NULL DEFAULT 'general',
    summary TEXT NOT NULL DEFAULT '',
    content_md TEXT NOT NULL DEFAULT '',
    link_label TEXT NOT NULL DEFAULT '',
    link_url TEXT NOT NULL DEFAULT '',
    machine_host TEXT NOT NULL DEFAULT '',
    machine_port TEXT NOT NULL DEFAULT '',
    machine_username TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '',
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    updated_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_category_updated_at
    ON wiki_pages (category, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_parent_title
    ON wiki_pages (parent_id, title ASC);

CREATE TABLE IF NOT EXISTS wiki_page_revisions (
    id TEXT PRIMARY KEY,
    page_id TEXT NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    title TEXT NOT NULL,
    page_type TEXT NOT NULL,
    category TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    content_md TEXT NOT NULL DEFAULT '',
    link_label TEXT NOT NULL DEFAULT '',
    link_url TEXT NOT NULL DEFAULT '',
    machine_host TEXT NOT NULL DEFAULT '',
    machine_port TEXT NOT NULL DEFAULT '',
    machine_username TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '',
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (page_id, version)
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_revisions_page_version
    ON wiki_page_revisions (page_id, version DESC);

INSERT INTO wiki_pages (
    id,
    title,
    page_type,
    category,
    summary,
    content_md,
    link_label,
    link_url,
    machine_host,
    machine_port,
    machine_username,
    tags
)
VALUES (
    'wiki-welcome',
    '资料库使用说明',
    'runbook',
    '操作手册',
    '记录机器信息、外部链接、操作步骤和故障处理经验。',
    '# 资料库使用说明

## 适合记录

- 机器信息和登录入口
- 常用操作手册
- 故障原因和处理步骤
- 外部监控、面板、文档链接

## 建议

- 机器密码先谨慎记录，后续可升级为加密字段
- 重要操作步骤写清楚前置条件和回滚方式
- 故障处理页面保留现象、原因、处理和验证结果',
    '内部文档入口',
    'https://example.com',
    '10.0.0.10',
    '22',
    'root',
    'wiki,runbook,server'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO wiki_page_revisions (
    id,
    page_id,
    version,
    title,
    page_type,
    category,
    summary,
    content_md,
    link_label,
    link_url,
    machine_host,
    machine_port,
    machine_username,
    tags
)
SELECT
    'wiki-welcome-r1',
    id,
    1,
    title,
    page_type,
    category,
    summary,
    content_md,
    link_label,
    link_url,
    machine_host,
    machine_port,
    machine_username,
    tags
FROM wiki_pages
WHERE id = 'wiki-welcome'
ON CONFLICT (page_id, version) DO NOTHING;
