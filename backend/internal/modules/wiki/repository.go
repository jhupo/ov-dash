package wiki

import (
	"context"
	"encoding/json"
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	resources, err := r.listResources(ctx, "")
	if err != nil {
		return nil, err
	}
	attachResources(items, resources)
	return items, nil
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
		       tags,
		       COALESCE(created_by, '') AS created_by,
		       COALESCE(updated_by, '') AS updated_by,
		       created_at,
		       updated_at
		FROM wiki_pages
		WHERE id = $1
	`, id)
	page, err := scanPage(row)
	if err != nil {
		return Page{}, err
	}
	page.Resources, err = r.listResources(ctx, page.ID)
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

func (r *Repository) Create(ctx context.Context, input SavePageInput) (Page, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Page{}, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		INSERT INTO wiki_pages (
			id,
			parent_id,
			title,
			page_type,
			category,
			summary,
			content_md,
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
			NULLIF($9, ''),
			NULLIF($9, '')
		)
		RETURNING id,
		          COALESCE(parent_id, '') AS parent_id,
		          title,
		          page_type,
		          category,
		          summary,
		          content_md,
		          tags,
		          COALESCE(created_by, '') AS created_by,
		          COALESCE(updated_by, '') AS updated_by,
		          created_at,
		          updated_at
	`, input.ID, input.ParentID, input.Title, input.PageType, input.Category, input.Summary, input.ContentMD, input.Tags, input.ActorID)

	page, err := scanPage(row)
	if err != nil {
		return Page{}, err
	}
	page.Resources, err = r.replaceResources(ctx, tx, page.ID, input.Resources)
	if err != nil {
		return Page{}, err
	}
	if err := r.insertRevision(ctx, tx, page, input.ActorID); err != nil {
		return Page{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Page{}, err
	}

	page.Resources, err = r.listResources(ctx, page.ID)
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

func (r *Repository) Update(ctx context.Context, input SavePageInput) (Page, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Page{}, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		UPDATE wiki_pages
		SET parent_id = NULLIF($2, ''),
		    title = $3,
		    page_type = $4,
		    category = $5,
		    summary = $6,
		    content_md = $7,
		    tags = $8,
		    updated_by = NULLIF($9, ''),
		    updated_at = now()
		WHERE id = $1
		RETURNING id,
		          COALESCE(parent_id, '') AS parent_id,
		          title,
		          page_type,
		          category,
		          summary,
		          content_md,
		          tags,
		          COALESCE(created_by, '') AS created_by,
		          COALESCE(updated_by, '') AS updated_by,
		          created_at,
		          updated_at
	`, input.ID, input.ParentID, input.Title, input.PageType, input.Category, input.Summary, input.ContentMD, input.Tags, input.ActorID)

	page, err := scanPage(row)
	if err != nil {
		return Page{}, err
	}
	page.Resources, err = r.replaceResources(ctx, tx, page.ID, input.Resources)
	if err != nil {
		return Page{}, err
	}
	if err := r.insertRevision(ctx, tx, page, input.ActorID); err != nil {
		return Page{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Page{}, err
	}

	page.Resources, err = r.listResources(ctx, page.ID)
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM wiki_pages WHERE id = $1`, id)
	return err
}

func (r *Repository) CreateAttachment(ctx context.Context, input SaveAttachmentInput) (Attachment, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO wiki_attachments (
			id,
			page_id,
			original_name,
			storage_path,
			content_type,
			size_bytes,
			created_by
		)
		VALUES (
			$1,
			NULLIF($2, ''),
			$3,
			$4,
			$5,
			$6,
			NULLIF($7, '')
		)
		RETURNING id,
		          COALESCE(page_id, '') AS page_id,
		          original_name,
		          storage_path,
		          content_type,
		          size_bytes,
		          COALESCE(created_by, '') AS created_by,
		          created_at
	`, input.ID, input.PageID, input.OriginalName, input.StoragePath, input.ContentType, input.SizeBytes, input.ActorID)

	return scanAttachment(row)
}

func (r *Repository) GetAttachment(ctx context.Context, id string) (Attachment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id,
		       COALESCE(page_id, '') AS page_id,
		       original_name,
		       storage_path,
		       content_type,
		       size_bytes,
		       COALESCE(created_by, '') AS created_by,
		       created_at
		FROM wiki_attachments
		WHERE id = $1
	`, id)

	return scanAttachment(row)
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
		       tags,
		       resources_json,
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
		var resourcesJSON []byte
		if err := rows.Scan(
			&item.ID,
			&item.PageID,
			&item.Version,
			&item.Title,
			&item.PageType,
			&item.Category,
			&item.Summary,
			&item.ContentMD,
			&item.Tags,
			&resourcesJSON,
			&item.CreatedBy,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if len(resourcesJSON) > 0 {
			if err := json.Unmarshal(resourcesJSON, &item.Resources); err != nil {
				return nil, err
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) replaceResources(ctx context.Context, tx pgx.Tx, pageID string, resources []SaveResourceInput) ([]Resource, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM wiki_page_resources WHERE page_id = $1`, pageID); err != nil {
		return nil, err
	}

	items := make([]Resource, 0, len(resources))
	for index, resource := range resources {
		resourceID := resource.ID
		if resourceID == "" {
			id, err := randomID()
			if err != nil {
				return nil, err
			}
			resourceID = id
		}
		sortOrder := resource.SortOrder
		if sortOrder == 0 {
			sortOrder = index + 1
		}

		row := tx.QueryRow(ctx, `
			INSERT INTO wiki_page_resources (
				id,
				page_id,
				resource_type,
				title,
				host,
				port,
				url,
				username,
				password,
				note,
				sort_order
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id,
			          page_id,
			          resource_type,
			          title,
			          host,
			          port,
			          url,
			          username,
			          password,
			          note,
			          sort_order,
			          created_at,
			          updated_at
		`, resourceID, pageID, resource.ResourceType, resource.Title, resource.Host, resource.Port, resource.URL, resource.Username, resource.Password, resource.Note, sortOrder)
		var item Resource
		err := row.Scan(
			&item.ID,
			&item.PageID,
			&item.ResourceType,
			&item.Title,
			&item.Host,
			&item.Port,
			&item.URL,
			&item.Username,
			&item.Password,
			&item.Note,
			&item.SortOrder,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

func (r *Repository) listResources(ctx context.Context, pageID string) ([]Resource, error) {
	query := `
		SELECT id,
		       page_id,
		       resource_type,
		       title,
		       host,
		       port,
		       url,
		       username,
		       password,
		       note,
		       sort_order,
		       created_at,
		       updated_at
		FROM wiki_page_resources
	`
	args := []any{}
	if pageID != "" {
		query += ` WHERE page_id = $1`
		args = append(args, pageID)
	}
	query += ` ORDER BY page_id ASC, sort_order ASC, title ASC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Resource, 0)
	for rows.Next() {
		var item Resource
		if err := rows.Scan(
			&item.ID,
			&item.PageID,
			&item.ResourceType,
			&item.Title,
			&item.Host,
			&item.Port,
			&item.URL,
			&item.Username,
			&item.Password,
			&item.Note,
			&item.SortOrder,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) insertRevision(ctx context.Context, tx pgx.Tx, page Page, actorID string) error {
	revisionID, err := randomID()
	if err != nil {
		return err
	}
	resourcesJSONBytes, err := json.Marshal(page.Resources)
	if err != nil {
		return err
	}
	resourcesJSON := string(resourcesJSONBytes)

	_, err = tx.Exec(ctx, `
		INSERT INTO wiki_page_revisions (
			id,
			page_id,
			version,
			title,
			page_type,
			category,
			summary,
			content_md,
			tags,
			resources_json,
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
			$9::jsonb,
			NULLIF($10, '')
		)
	`, revisionID, page.ID, page.Title, page.PageType, page.Category, page.Summary, page.ContentMD, page.Tags, resourcesJSON, actorID)
	return err
}

func attachResources(pages []Page, resources []Resource) {
	byPage := make(map[string][]Resource)
	for _, resource := range resources {
		byPage[resource.PageID] = append(byPage[resource.PageID], resource)
	}
	for index := range pages {
		pages[index].Resources = byPage[pages[index].ID]
	}
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
		&item.Tags,
		&item.CreatedBy,
		&item.UpdatedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func scanAttachment(row pageScanner) (Attachment, error) {
	var item Attachment
	err := row.Scan(
		&item.ID,
		&item.PageID,
		&item.OriginalName,
		&item.StoragePath,
		&item.ContentType,
		&item.SizeBytes,
		&item.CreatedBy,
		&item.CreatedAt,
	)
	return item, err
}
