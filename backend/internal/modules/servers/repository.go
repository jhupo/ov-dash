package servers

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

func (r *Repository) List(ctx context.Context) ([]Connection, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, group_name, region, host, port, username, auth_type,
		       password, private_key, passphrase, created_at, updated_at
		FROM server_connections
		ORDER BY group_name ASC, name ASC, host ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Connection, 0)
	for rows.Next() {
		var item Connection
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.GroupName,
			&item.Region,
			&item.Host,
			&item.Port,
			&item.Username,
			&item.AuthType,
			&item.Password,
			&item.PrivateKey,
			&item.Passphrase,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Upsert(ctx context.Context, input SaveInput) (Connection, error) {
	var item Connection
	err := r.db.QueryRow(ctx, `
		INSERT INTO server_connections (
			id, name, group_name, region, host, port, username, auth_type,
			password, private_key, passphrase
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, ''), COALESCE($10, ''), COALESCE($11, ''))
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			group_name = EXCLUDED.group_name,
			region = EXCLUDED.region,
			host = EXCLUDED.host,
			port = EXCLUDED.port,
			username = EXCLUDED.username,
			auth_type = EXCLUDED.auth_type,
			password = CASE
				WHEN $12 THEN ''
				WHEN $9::text IS NULL THEN server_connections.password
				ELSE EXCLUDED.password
			END,
			private_key = CASE
				WHEN $12 THEN ''
				WHEN $10::text IS NULL THEN server_connections.private_key
				ELSE EXCLUDED.private_key
			END,
			passphrase = CASE
				WHEN $12 THEN ''
				WHEN $11::text IS NULL THEN server_connections.passphrase
				ELSE EXCLUDED.passphrase
			END,
			updated_at = now()
		RETURNING id, name, group_name, region, host, port, username, auth_type,
		          password, private_key, passphrase, created_at, updated_at
	`,
		input.ID,
		input.Name,
		input.GroupName,
		input.Region,
		input.Host,
		input.Port,
		input.Username,
		input.AuthType,
		input.Password,
		input.PrivateKey,
		input.Passphrase,
		input.ClearSecret,
	).Scan(
		&item.ID,
		&item.Name,
		&item.GroupName,
		&item.Region,
		&item.Host,
		&item.Port,
		&item.Username,
		&item.AuthType,
		&item.Password,
		&item.PrivateKey,
		&item.Passphrase,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM server_connections WHERE id = $1`, id)
	return err
}
