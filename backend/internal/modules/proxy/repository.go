package proxy

import (
	"context"

	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/platform/secret"
)

type Repository struct {
	db      *db.Pool
	secrets *secret.Store
}

func NewRepository(db *db.Pool) *Repository {
	return &Repository{db: db}
}

func NewRepositoryWithSecrets(db *db.Pool, secrets *secret.Store) *Repository {
	return &Repository{db: db, secrets: secrets}
}

func (r *Repository) Get(ctx context.Context) (Settings, error) {
	var settings Settings
	err := r.db.QueryRow(ctx, `
		INSERT INTO proxy_settings (id)
		VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
		RETURNING id, enabled, scheme, host, port, username, password, password_secret_id, updated_at
	`, DefaultSettingsID).Scan(
		&settings.ID,
		&settings.Enabled,
		&settings.Scheme,
		&settings.Host,
		&settings.Port,
		&settings.Username,
		&settings.Password,
		&settings.PasswordSecretID,
		&settings.UpdatedAt,
	)
	if err == nil {
		settings.Password = r.resolveSecret(ctx, settings.PasswordSecretID, settings.Password)
	}
	return settings, err
}

func (r *Repository) Update(ctx context.Context, input UpdateSettingsInput) (Settings, error) {
	current, err := r.Get(ctx)
	if err != nil {
		return Settings{}, err
	}

	password := current.Password
	passwordSecretID := current.PasswordSecretID
	if input.ClearPassword {
		password = ""
		if passwordSecretID != "" && r.secrets != nil {
			_ = r.secrets.Delete(ctx, passwordSecretID)
		}
		passwordSecretID = ""
	} else if input.Password != nil {
		password = *input.Password
		if r.secrets != nil && password != "" {
			id, err := r.secrets.Put(ctx, "proxy_settings:"+DefaultSettingsID, "password", password)
			if err != nil {
				return Settings{}, err
			}
			passwordSecretID = id
			password = ""
		}
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
			password_secret_id = $7,
			updated_at = now()
		WHERE id = $1
		RETURNING id, enabled, scheme, host, port, username, password, password_secret_id, updated_at
	`, DefaultSettingsID, input.Enabled, input.Host, input.Port, input.Username, password, passwordSecretID).Scan(
		&settings.ID,
		&settings.Enabled,
		&settings.Scheme,
		&settings.Host,
		&settings.Port,
		&settings.Username,
		&settings.Password,
		&settings.PasswordSecretID,
		&settings.UpdatedAt,
	)
	if err == nil {
		settings.Password = r.resolveSecret(ctx, settings.PasswordSecretID, settings.Password)
	}
	return settings, err
}

func (r *Repository) resolveSecret(ctx context.Context, secretID string, fallback string) string {
	if secretID == "" || r.secrets == nil {
		return fallback
	}
	value, err := r.secrets.Get(ctx, secretID)
	if err != nil {
		return fallback
	}
	return value
}
