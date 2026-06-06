package users

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

func (r *Repository) List(ctx context.Context) ([]User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, first_name, last_name, username, email, phone_number, status, role, created_at, updated_at
		FROM users
		ORDER BY created_at DESC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]User, 0)
	for rows.Next() {
		var item User
		if err := rows.Scan(
			&item.ID,
			&item.FirstName,
			&item.LastName,
			&item.Username,
			&item.Email,
			&item.PhoneNumber,
			&item.Status,
			&item.Role,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
