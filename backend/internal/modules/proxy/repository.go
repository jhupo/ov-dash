package proxy

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

func (r *Repository) Get(ctx context.Context) (Settings, error) {
	var settings Settings
	err := r.db.QueryRow(ctx, `
		INSERT INTO proxy_settings (id)
		VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
		RETURNING id, enabled, scheme, host, port, username, password, updated_at
	`, DefaultSettingsID).Scan(
		&settings.ID,
		&settings.Enabled,
		&settings.Scheme,
		&settings.Host,
		&settings.Port,
		&settings.Username,
		&settings.Password,
		&settings.UpdatedAt,
	)
	return settings, err
}

func (r *Repository) Update(ctx context.Context, input UpdateSettingsInput) (Settings, error) {
	current, err := r.Get(ctx)
	if err != nil {
		return Settings{}, err
	}

	password := current.Password
	if input.ClearPassword {
		password = ""
	} else if input.Password != nil {
		password = *input.Password
	}

	var settings Settings
	err = r.db.QueryRow(ctx, `
		UPDATE proxy_settings
		SET enabled = $2,
			scheme = 'socks5',
			host = $3,
			port = $4,
			username = $5,
			password = $6,
			updated_at = now()
		WHERE id = $1
		RETURNING id, enabled, scheme, host, port, username, password, updated_at
	`, DefaultSettingsID, input.Enabled, input.Host, input.Port, input.Username, password).Scan(
		&settings.ID,
		&settings.Enabled,
		&settings.Scheme,
		&settings.Host,
		&settings.Port,
		&settings.Username,
		&settings.Password,
		&settings.UpdatedAt,
	)
	return settings, err
}
