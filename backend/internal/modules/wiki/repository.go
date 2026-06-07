package wiki

import (
	"context"
	"errors"

	"ov-dash/backend/internal/db"

	"github.com/jackc/pgx/v5"
)

type Repository struct {
	db *db.Pool
}

func NewRepository(db *db.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context) ([]Page, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id,
		       COALESCE(parent_id, '') AS parent_id,
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
		       tags,
		       COALESCE(created_by, '') AS created_by,
		       COALESCE(updated_by, '') AS updated_by,
		       created_at,
		       updated_at
		FROM wiki_pages
		ORDER BY category ASC, title ASC, updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Page, 0)
	for rows.Next() {
		item, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Page, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id,
		       COALESCE(parent_id, '') AS parent_id,
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
		       tags,
		       COALESCE(created_by, '') AS created_by,
		       COALESCE(updated_by, '') AS updated_by,
		       created_at,
		       updated_at
		FROM wiki_pages
		WHERE id = $1
	`, id)
	return scanPage(row)
}

func (r *Repository) Create(ctx context.Context, input SavePageInput) (Page, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO wiki_pages (
			id,
			parent_id,
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
			tags,
			created_by,
			updated_by
		)
		VALUES (
			$1,
			NULLIF($2, ''),
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10,
			$11,
			$12,
			$13,
			NULLIF($14, ''),
			NULLIF($14, '')
		)
		RETURNING id,
		          COALESCE(parent_id, '') AS parent_id,
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
		          tags,
		          COALESCE(created_by, '') AS created_by,
		          COALESCE(updated_by, '') AS updated_by,
		          created_at,
		          updated_at
	`, input.ID, input.ParentID, input.Title, input.PageType, input.Category, input.Summary, input.ContentMD, input.LinkLabel, input.LinkURL, input.MachineHost, input.MachinePort, input.MachineUsername, input.Tags, input.ActorID)

	page, err := scanPage(row)
	if err != nil {
		return Page{}, err
	}
	return page, r.insertRevision(ctx, page, input.ActorID)
}

func (r *Repository) Update(ctx context.Context, input SavePageInput) (Page, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE wiki_pages
		SET parent_id = NULLIF($2, ''),
		    title = $3,
		    page_type = $4,
		    category = $5,
		    summary = $6,
		    content_md = $7,
		    link_label = $8,
		    link_url = $9,
		    machine_host = $10,
		    machine_port = $11,
		    machine_username = $12,
		    tags = $13,
		    updated_by = NULLIF($14, ''),
		    updated_at = now()
		WHERE id = $1
		RETURNING id,
		          COALESCE(parent_id, '') AS parent_id,
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
		          tags,
		          COALESCE(created_by, '') AS created_by,
		          COALESCE(updated_by, '') AS updated_by,
		          created_at,
		          updated_at
	`, input.ID, input.ParentID, input.Title, input.PageType, input.Category, input.Summary, input.ContentMD, input.LinkLabel, input.LinkURL, input.MachineHost, input.MachinePort, input.MachineUsername, input.Tags, input.ActorID)

	page, err := scanPage(row)
	if err != nil {
		return Page{}, err
	}
	return page, r.insertRevision(ctx, page, input.ActorID)
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM wiki_pages WHERE id = $1`, id)
	return err
}

func (r *Repository) ListRevisions(ctx context.Context, pageID string) ([]Revision, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id,
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
		       tags,
		       COALESCE(created_by, '') AS created_by,
		       created_at
		FROM wiki_page_revisions
		WHERE page_id = $1
		ORDER BY version DESC
	`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Revision, 0)
	for rows.Next() {
		var item Revision
		if err := rows.Scan(
			&item.ID,
			&item.PageID,
			&item.Version,
			&item.Title,
			&item.PageType,
			&item.Category,
			&item.Summary,
			&item.ContentMD,
			&item.LinkLabel,
			&item.LinkURL,
			&item.MachineHost,
			&item.MachinePort,
			&item.MachineUsername,
			&item.Tags,
			&item.CreatedBy,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) insertRevision(ctx context.Context, page Page, actorID string) error {
	revisionID, err := randomID()
	if err != nil {
		return err
	}

	_, err = r.db.Exec(ctx, `
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
			tags,
			created_by
		)
		VALUES (
			$1,
			$2,
			(SELECT COALESCE(MAX(version), 0) + 1 FROM wiki_page_revisions WHERE page_id = $2),
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10,
			$11,
			$12,
			$13,
			NULLIF($14, '')
		)
	`, revisionID, page.ID, page.Title, page.PageType, page.Category, page.Summary, page.ContentMD, page.LinkLabel, page.LinkURL, page.MachineHost, page.MachinePort, page.MachineUsername, page.Tags, actorID)
	return err
}

func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

type pageScanner interface {
	Scan(dest ...any) error
}

func scanPage(row pageScanner) (Page, error) {
	var item Page
	err := row.Scan(
		&item.ID,
		&item.ParentID,
		&item.Title,
		&item.PageType,
		&item.Category,
		&item.Summary,
		&item.ContentMD,
		&item.LinkLabel,
		&item.LinkURL,
		&item.MachineHost,
		&item.MachinePort,
		&item.MachineUsername,
		&item.Tags,
		&item.CreatedBy,
		&item.UpdatedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}
