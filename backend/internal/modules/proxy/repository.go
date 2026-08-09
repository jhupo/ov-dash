package proxy

import (
	"context"
	"errors"
	"fmt"

	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/platform/secret"

	"github.com/jackc/pgx/v5"
)

type repositoryDB interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type secretStore interface {
	PutTx(context.Context, pgx.Tx, string, string, string) (string, error)
	Get(context.Context, string) (string, error)
	GetTx(context.Context, pgx.Tx, string) (string, error)
	DeleteTx(context.Context, pgx.Tx, string) error
}

type Repository struct {
	db      repositoryDB
	secrets secretStore
}

func NewRepository(database *db.Pool, secrets *secret.Store) *Repository {
	var secretBackend secretStore
	if secrets != nil {
		secretBackend = secrets
	}
	return &Repository{db: database, secrets: secretBackend}
}

func (r *Repository) Get(ctx context.Context) (Settings, error) {
	var settings Settings
	err := r.db.QueryRow(ctx, `
		INSERT INTO proxy_settings (id)
		VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
		RETURNING id, enabled, scheme, host, port, username, password_secret_id, updated_at
	`, DefaultSettingsID).Scan(
		&settings.ID,
		&settings.Enabled,
		&settings.Scheme,
		&settings.Host,
		&settings.Port,
		&settings.Username,
		&settings.PasswordSecretID,
		&settings.UpdatedAt,
	)
	if err != nil {
		return Settings{}, err
	}
	if err := r.resolvePassword(ctx, &settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func (r *Repository) Update(ctx context.Context, input UpdateSettingsInput) (Settings, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Settings{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO proxy_settings (id)
		VALUES ($1)
		ON CONFLICT (id) DO NOTHING
	`, DefaultSettingsID); err != nil {
		return Settings{}, err
	}

	var currentSecretID string
	if err := tx.QueryRow(ctx, `
		SELECT password_secret_id
		FROM proxy_settings
		WHERE id = $1
		FOR UPDATE
	`, DefaultSettingsID).Scan(&currentSecretID); err != nil {
		return Settings{}, err
	}

	passwordSecretID, err := r.updatePassword(ctx, tx, currentSecretID, input.Password, input.ClearPassword)
	if err != nil {
		return Settings{}, err
	}

	var settings Settings
	err = tx.QueryRow(ctx, `
		UPDATE proxy_settings
		SET enabled = $2,
			scheme = 'socks5',
			host = $3,
			port = $4,
			username = $5,
			password = '',
			password_secret_id = $6,
			updated_at = now()
		WHERE id = $1
		RETURNING id, enabled, scheme, host, port, username, password_secret_id, updated_at
	`, DefaultSettingsID, input.Enabled, input.Host, input.Port, input.Username, passwordSecretID).Scan(
		&settings.ID,
		&settings.Enabled,
		&settings.Scheme,
		&settings.Host,
		&settings.Port,
		&settings.Username,
		&settings.PasswordSecretID,
		&settings.UpdatedAt,
	)
	if err != nil {
		return Settings{}, err
	}
	if err := r.resolvePasswordTx(ctx, tx, &settings); err != nil {
		return Settings{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func (r *Repository) updatePassword(
	ctx context.Context,
	tx pgx.Tx,
	currentSecretID string,
	password *string,
	clear bool,
) (string, error) {
	if !clear && password == nil {
		return currentSecretID, nil
	}
	if r.secrets == nil {
		return "", errors.New("proxy secret store is unavailable")
	}
	if clear || *password == "" {
		if err := r.secrets.DeleteTx(ctx, tx, currentSecretID); err != nil {
			return "", fmt.Errorf("delete proxy password secret: %w", err)
		}
		return "", nil
	}

	secretID, err := r.secrets.PutTx(ctx, tx, "proxy_settings:"+DefaultSettingsID, "password", *password)
	if err != nil {
		return "", fmt.Errorf("store proxy password secret: %w", err)
	}
	if currentSecretID != "" && currentSecretID != secretID {
		if err := r.secrets.DeleteTx(ctx, tx, currentSecretID); err != nil {
			return "", fmt.Errorf("delete replaced proxy password secret: %w", err)
		}
	}
	return secretID, nil
}

func (r *Repository) resolvePassword(ctx context.Context, settings *Settings) error {
	if settings.PasswordSecretID == "" {
		settings.Password = ""
		return nil
	}
	if r.secrets == nil {
		return errors.New("proxy secret store is unavailable")
	}
	password, err := r.secrets.Get(ctx, settings.PasswordSecretID)
	if err != nil {
		return fmt.Errorf("decrypt proxy password secret %q: %w", settings.PasswordSecretID, err)
	}
	settings.Password = password
	return nil
}

func (r *Repository) resolvePasswordTx(ctx context.Context, tx pgx.Tx, settings *Settings) error {
	if settings.PasswordSecretID == "" {
		settings.Password = ""
		return nil
	}
	if r.secrets == nil {
		return errors.New("proxy secret store is unavailable")
	}
	password, err := r.secrets.GetTx(ctx, tx, settings.PasswordSecretID)
	if err != nil {
		return fmt.Errorf("decrypt proxy password secret %q: %w", settings.PasswordSecretID, err)
	}
	settings.Password = password
	return nil
}
