package servers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/platform/secret"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type repositoryDB interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type secretStore interface {
	PutTx(context.Context, pgx.Tx, string, string, string) (string, error)
	Get(context.Context, string) (string, error)
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

func (r *Repository) List(ctx context.Context) ([]Connection, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, group_name, region, host, port, username, auth_type,
		       password_secret_id, private_key_secret_id,
		       expires_at, created_at, updated_at
		FROM server_connections
		ORDER BY group_name ASC, name ASC, host ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Connection, 0)
	for rows.Next() {
		item, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id string) (Connection, error) {
	return scanConnection(r.db.QueryRow(ctx, `
		SELECT id, name, group_name, region, host, port, username, auth_type,
		       password_secret_id, private_key_secret_id,
		       expires_at, created_at, updated_at
		FROM server_connections
		WHERE id = $1
	`, id))
}

func (r *Repository) GetWithCredentials(ctx context.Context, id string) (Connection, error) {
	item, err := r.Get(ctx, id)
	if err != nil {
		return Connection{}, err
	}
	if item.PasswordSecretID != "" {
		if r.secrets == nil {
			return Connection{}, errors.New("server secret store is unavailable")
		}
		item.Password, err = r.secrets.Get(ctx, item.PasswordSecretID)
		if err != nil {
			return Connection{}, fmt.Errorf("decrypt server password secret %q: %w", item.PasswordSecretID, err)
		}
	}
	if item.PrivateKeySecretID != "" {
		if r.secrets == nil {
			return Connection{}, errors.New("server secret store is unavailable")
		}
		item.PrivateKey, err = r.secrets.Get(ctx, item.PrivateKeySecretID)
		if err != nil {
			return Connection{}, fmt.Errorf("decrypt server private key secret %q: %w", item.PrivateKeySecretID, err)
		}
	}
	return item, nil
}

func (r *Repository) Upsert(ctx context.Context, input SaveInput) (Connection, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Connection{}, err
	}
	defer tx.Rollback(ctx)

	var currentPasswordSecretID string
	var currentPrivateKeySecretID string
	err = tx.QueryRow(ctx, `
		SELECT password_secret_id, private_key_secret_id
		FROM server_connections
		WHERE id = $1
		FOR UPDATE
	`, input.ID).Scan(&currentPasswordSecretID, &currentPrivateKeySecretID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, err
	}

	passwordSecretID, err := r.updateCredential(
		ctx, tx, input.ID, "password", currentPasswordSecretID, input.Password, input.ClearSecret,
	)
	if err != nil {
		return Connection{}, err
	}
	privateKeySecretID, err := r.updateCredential(
		ctx, tx, input.ID, "private_key", currentPrivateKeySecretID, input.PrivateKey, input.ClearSecret,
	)
	if err != nil {
		return Connection{}, err
	}

	item, err := scanConnection(tx.QueryRow(ctx, `
		INSERT INTO server_connections (
			id, name, group_name, region, host, port, username, auth_type,
			password, private_key, password_secret_id, private_key_secret_id, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '', '', $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			group_name = EXCLUDED.group_name,
			region = EXCLUDED.region,
			host = EXCLUDED.host,
			port = EXCLUDED.port,
			username = EXCLUDED.username,
			auth_type = EXCLUDED.auth_type,
			expires_at = EXCLUDED.expires_at,
			password = '',
			private_key = '',
			password_secret_id = EXCLUDED.password_secret_id,
			private_key_secret_id = EXCLUDED.private_key_secret_id,
			updated_at = now()
		RETURNING id, name, group_name, region, host, port, username, auth_type,
		          password_secret_id, private_key_secret_id,
		          expires_at, created_at, updated_at
	`,
		input.ID,
		input.Name,
		input.GroupName,
		input.Region,
		input.Host,
		input.Port,
		input.Username,
		input.AuthType,
		passwordSecretID,
		privateKeySecretID,
		input.ExpiresAt,
	))
	if err != nil {
		return Connection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Connection{}, err
	}
	return item, nil
}

func (r *Repository) updateCredential(
	ctx context.Context,
	tx pgx.Tx,
	serverID string,
	name string,
	currentSecretID string,
	value *string,
	clear bool,
) (string, error) {
	if !clear && value == nil {
		return currentSecretID, nil
	}
	if r.secrets == nil {
		return "", errors.New("server secret store is unavailable")
	}
	if clear || *value == "" {
		if err := r.secrets.DeleteTx(ctx, tx, currentSecretID); err != nil {
			return "", fmt.Errorf("delete server %s secret: %w", name, err)
		}
		return "", nil
	}

	secretID, err := r.secrets.PutTx(ctx, tx, serverCredentialScope(serverID), name, *value)
	if err != nil {
		return "", fmt.Errorf("store server %s secret: %w", name, err)
	}
	if currentSecretID != "" && currentSecretID != secretID {
		if err := r.secrets.DeleteTx(ctx, tx, currentSecretID); err != nil {
			return "", fmt.Errorf("delete replaced server %s secret: %w", name, err)
		}
	}
	return secretID, nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var passwordSecretID string
	var privateKeySecretID string
	err = tx.QueryRow(ctx, `
		SELECT password_secret_id, private_key_secret_id
		FROM server_connections
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&passwordSecretID, &privateKeySecretID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM server_connections WHERE id = $1`, id); err != nil {
		return err
	}
	if (passwordSecretID != "" || privateKeySecretID != "") && r.secrets == nil {
		return errors.New("server secret store is unavailable")
	}
	if passwordSecretID != "" {
		if err := r.secrets.DeleteTx(ctx, tx, passwordSecretID); err != nil {
			return fmt.Errorf("delete server password secret: %w", err)
		}
	}
	if privateKeySecretID != "" {
		if err := r.secrets.DeleteTx(ctx, tx, privateKeySecretID); err != nil {
			return fmt.Errorf("delete server private key secret: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) VerifyOrRememberSSHHostKey(ctx context.Context, key SSHHostKey) error {
	if r == nil || r.db == nil || strings.TrimSpace(key.ServerID) == "" ||
		strings.TrimSpace(key.Algorithm) == "" || strings.TrimSpace(key.PublicKey) == "" ||
		strings.TrimSpace(key.Fingerprint) == "" {
		return errSSHHostKeyVerificationFailed
	}
	tag, err := r.db.Exec(ctx, `
		INSERT INTO server_ssh_host_keys (server_id, algorithm, public_key, fingerprint)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (server_id) DO NOTHING
	`, key.ServerID, key.Algorithm, key.PublicKey, key.Fingerprint)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}

	var stored SSHHostKey
	err = r.db.QueryRow(ctx, `
		SELECT server_id, algorithm, public_key, fingerprint
		FROM server_ssh_host_keys
		WHERE server_id = $1
	`, key.ServerID).Scan(&stored.ServerID, &stored.Algorithm, &stored.PublicKey, &stored.Fingerprint)
	if err != nil {
		return err
	}
	if stored.Algorithm != key.Algorithm || stored.PublicKey != key.PublicKey || stored.Fingerprint != key.Fingerprint {
		return ErrSSHHostKeyChanged
	}
	tag, err = r.db.Exec(ctx, `
		UPDATE server_ssh_host_keys
		SET last_verified_at = now()
		WHERE server_id = $1
		  AND algorithm = $2
		  AND public_key = $3
		  AND fingerprint = $4
	`, key.ServerID, key.Algorithm, key.PublicKey, key.Fingerprint)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrSSHHostKeyChanged
	}
	return nil
}

type connectionScanner interface {
	Scan(dest ...any) error
}

func scanConnection(row connectionScanner) (Connection, error) {
	var item Connection
	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.GroupName,
		&item.Region,
		&item.Host,
		&item.Port,
		&item.Username,
		&item.AuthType,
		&item.PasswordSecretID,
		&item.PrivateKeySecretID,
		&item.ExpiresAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func serverCredentialScope(id string) string {
	return "server_connections:" + id
}

func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
