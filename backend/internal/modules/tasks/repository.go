package tasks

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

func (r *Repository) List(ctx context.Context) ([]Task, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, title, status, label, priority, description, assignee, due_date, created_at, updated_at
		FROM tasks
		ORDER BY created_at DESC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Task, 0)
	for rows.Next() {
		var item Task
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.Status,
			&item.Label,
			&item.Priority,
			&item.Description,
			&item.Assignee,
			&item.DueDate,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
