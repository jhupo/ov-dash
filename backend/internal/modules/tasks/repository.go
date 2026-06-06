package tasks

import (
	"context"
	"time"

	"ov-dash/backend/internal/db"
)

type Repository struct {
	db *db.Pool
}

type UpsertInput struct {
	ID          string
	Title       string
	Status      string
	Label       string
	Priority    string
	Description string
	Assignee    string
	DueDate     *time.Time
}

func NewRepository(db *db.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Upsert(ctx context.Context, input UpsertInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO tasks (id, title, status, label, priority, description, assignee, due_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			status = EXCLUDED.status,
			label = EXCLUDED.label,
			priority = EXCLUDED.priority,
			description = EXCLUDED.description,
			assignee = EXCLUDED.assignee,
			due_date = EXCLUDED.due_date,
			updated_at = now()
	`, input.ID, input.Title, input.Status, input.Label, input.Priority, input.Description, input.Assignee, input.DueDate)
	return err
}

func (r *Repository) UpdateStatus(ctx context.Context, id string, status string, description string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE tasks
		SET status = $2,
		    description = $3,
		    updated_at = now()
		WHERE id = $1
	`, id, status, description)
	return err
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
