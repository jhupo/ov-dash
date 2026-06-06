package apps

import (
	"context"

	"ov-dash/backend/internal/db"
)

type Repository struct {
	db *db.Pool
}

func NewRepository(db *db.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context) ([]App, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, provider, description, connected, created_at, updated_at
		FROM apps
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]App, 0)
	for rows.Next() {
		var item App
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Provider,
			&item.Description,
			&item.Connected,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
