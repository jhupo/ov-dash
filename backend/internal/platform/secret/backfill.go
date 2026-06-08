package secret

import (
	"context"
	"fmt"

	"ov-dash/backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type BackfillSummary struct {
	ProxyPasswords    int
	ServerPasswords   int
	ServerPrivateKeys int
}

type credentialDB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type secretWriter interface {
	Put(ctx context.Context, scope string, name string, plaintext string) (string, error)
}

func BackfillLegacyCredentials(ctx context.Context, database *db.Pool, store *Store) (BackfillSummary, error) {
	if database == nil || store == nil {
		return BackfillSummary{}, nil
	}

	var summary BackfillSummary
	proxyCount, err := backfillProxyPasswords(ctx, database, store)
	if err != nil {
		return summary, err
	}
	summary.ProxyPasswords = proxyCount

	serverPasswords, serverPrivateKeys, err := backfillServerCredentials(ctx, database, store)
	if err != nil {
		return summary, err
	}
	summary.ServerPasswords = serverPasswords
	summary.ServerPrivateKeys = serverPrivateKeys
	return summary, nil
}

func backfillProxyPasswords(ctx context.Context, database credentialDB, store secretWriter) (int, error) {
	rows, err := database.Query(ctx, `
		SELECT id, password
		FROM proxy_settings
		WHERE password <> ''
		  AND password_secret_id = ''
	`)
	if err != nil {
		return 0, fmt.Errorf("list legacy proxy passwords: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id string
		var password string
		if err := rows.Scan(&id, &password); err != nil {
			return count, err
		}
		secretID, err := store.Put(ctx, "proxy_settings:"+id, "password", password)
		if err != nil {
			return count, err
		}
		if _, err := database.Exec(ctx, `
			UPDATE proxy_settings
			SET password = '',
			    password_secret_id = $2,
			    updated_at = now()
			WHERE id = $1
			  AND password_secret_id = ''
		`, id, secretID); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func backfillServerCredentials(ctx context.Context, database credentialDB, store secretWriter) (int, int, error) {
	rows, err := database.Query(ctx, `
		SELECT
			id,
			CASE WHEN password_secret_id = '' THEN password ELSE '' END,
			CASE WHEN private_key_secret_id = '' THEN private_key ELSE '' END
		FROM server_connections
		WHERE (password <> '' AND password_secret_id = '')
		   OR (private_key <> '' AND private_key_secret_id = '')
	`)
	if err != nil {
		return 0, 0, fmt.Errorf("list legacy server credentials: %w", err)
	}
	defer rows.Close()

	passwordCount := 0
	privateKeyCount := 0
	for rows.Next() {
		var id string
		var password string
		var privateKey string
		if err := rows.Scan(&id, &password, &privateKey); err != nil {
			return passwordCount, privateKeyCount, err
		}

		passwordSecretID := ""
		privateKeySecretID := ""
		if password != "" {
			secretID, err := store.Put(ctx, "server_connections:"+id, "password", password)
			if err != nil {
				return passwordCount, privateKeyCount, err
			}
			passwordSecretID = secretID
		}
		if privateKey != "" {
			secretID, err := store.Put(ctx, "server_connections:"+id, "private_key", privateKey)
			if err != nil {
				return passwordCount, privateKeyCount, err
			}
			privateKeySecretID = secretID
		}

		if _, err := database.Exec(ctx, `
			UPDATE server_connections
			SET password = CASE WHEN $2 <> '' THEN '' ELSE password END,
			    password_secret_id = CASE WHEN $2 <> '' THEN $2 ELSE password_secret_id END,
			    private_key = CASE WHEN $3 <> '' THEN '' ELSE private_key END,
			    private_key_secret_id = CASE WHEN $3 <> '' THEN $3 ELSE private_key_secret_id END,
			    updated_at = now()
			WHERE id = $1
		`, id, passwordSecretID, privateKeySecretID); err != nil {
			return passwordCount, privateKeyCount, err
		}
		if passwordSecretID != "" {
			passwordCount++
		}
		if privateKeySecretID != "" {
			privateKeyCount++
		}
	}
	return passwordCount, privateKeyCount, rows.Err()
}
