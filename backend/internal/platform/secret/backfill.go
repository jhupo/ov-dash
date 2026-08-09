package secret

import (
	"context"
	"fmt"

	"ov-dash/backend/internal/db"

	"github.com/jackc/pgx/v5"
)

type BackfillSummary struct {
	ProxyPasswords        int
	ServerPasswords       int
	ServerPrivateKeys     int
	TelegramBotTokens     int
	TelegramInboundTokens int
	WikiResourcePasswords int
}

type credentialDB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type secretWriter interface {
	PutTx(ctx context.Context, tx pgx.Tx, scope string, name string, plaintext string) (string, error)
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

	telegramBotTokens, telegramInboundTokens, err := backfillTelegramTokens(ctx, database, store)
	if err != nil {
		return summary, err
	}
	summary.TelegramBotTokens = telegramBotTokens
	summary.TelegramInboundTokens = telegramInboundTokens

	wikiResourcePasswords, err := backfillWikiResourcePasswords(ctx, database, store)
	if err != nil {
		return summary, err
	}
	summary.WikiResourcePasswords = wikiResourcePasswords
	return summary, nil
}

func backfillProxyPasswords(ctx context.Context, database credentialDB, store secretWriter) (int, error) {
	tx, err := database.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin proxy password backfill: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, password
		FROM proxy_settings
		WHERE password <> ''
		  AND password_secret_id = ''
		FOR UPDATE
	`)
	if err != nil {
		return 0, fmt.Errorf("list legacy proxy passwords: %w", err)
	}

	type legacyProxyPassword struct {
		id       string
		password string
	}
	legacy := make([]legacyProxyPassword, 0)
	for rows.Next() {
		var item legacyProxyPassword
		if err := rows.Scan(&item.id, &item.password); err != nil {
			rows.Close()
			return 0, err
		}
		legacy = append(legacy, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, item := range legacy {
		secretID, err := store.PutTx(ctx, tx, "proxy_settings:"+item.id, "password", item.password)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE proxy_settings
			SET password = '',
			    password_secret_id = $2,
			    updated_at = now()
			WHERE id = $1
			  AND password_secret_id = ''
		`, item.id, secretID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(legacy), nil
}

func backfillServerCredentials(ctx context.Context, database credentialDB, store secretWriter) (int, int, error) {
	tx, err := database.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin server credential backfill: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT
			id,
			CASE WHEN password_secret_id = '' THEN password ELSE '' END,
			CASE WHEN private_key_secret_id = '' THEN private_key ELSE '' END
		FROM server_connections
		WHERE (password <> '' AND password_secret_id = '')
		   OR (private_key <> '' AND private_key_secret_id = '')
		FOR UPDATE
	`)
	if err != nil {
		return 0, 0, fmt.Errorf("list legacy server credentials: %w", err)
	}

	type legacyServerCredentials struct {
		id         string
		password   string
		privateKey string
	}
	legacy := make([]legacyServerCredentials, 0)
	for rows.Next() {
		var item legacyServerCredentials
		if err := rows.Scan(&item.id, &item.password, &item.privateKey); err != nil {
			rows.Close()
			return 0, 0, err
		}
		legacy = append(legacy, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	passwordCount := 0
	privateKeyCount := 0
	for _, item := range legacy {
		passwordSecretID := ""
		privateKeySecretID := ""
		if item.password != "" {
			secretID, err := store.PutTx(ctx, tx, "server_connections:"+item.id, "password", item.password)
			if err != nil {
				return 0, 0, err
			}
			passwordSecretID = secretID
		}
		if item.privateKey != "" {
			secretID, err := store.PutTx(ctx, tx, "server_connections:"+item.id, "private_key", item.privateKey)
			if err != nil {
				return 0, 0, err
			}
			privateKeySecretID = secretID
		}

		if _, err := tx.Exec(ctx, `
			UPDATE server_connections
			SET password = CASE WHEN $2 <> '' THEN '' ELSE password END,
			    password_secret_id = CASE WHEN $2 <> '' THEN $2 ELSE password_secret_id END,
			    private_key = CASE WHEN $3 <> '' THEN '' ELSE private_key END,
			    private_key_secret_id = CASE WHEN $3 <> '' THEN $3 ELSE private_key_secret_id END,
			    updated_at = now()
			WHERE id = $1
		`, item.id, passwordSecretID, privateKeySecretID); err != nil {
			return 0, 0, err
		}
		if passwordSecretID != "" {
			passwordCount++
		}
		if privateKeySecretID != "" {
			privateKeyCount++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return passwordCount, privateKeyCount, nil
}

func backfillTelegramTokens(ctx context.Context, database credentialDB, store secretWriter) (int, int, error) {
	tx, err := database.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin telegram token backfill: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT
			id,
			bot_token,
			bot_token_secret_id,
			inbound_token,
			inbound_token_secret_id
		FROM telegram_notification_settings
		WHERE bot_token <> ''
		   OR inbound_token <> ''
	`)
	if err != nil {
		return 0, 0, fmt.Errorf("list legacy telegram tokens: %w", err)
	}

	type legacyTelegramTokens struct {
		id                   string
		botToken             string
		botTokenSecretID     string
		inboundToken         string
		inboundTokenSecretID string
	}
	legacy := make([]legacyTelegramTokens, 0)
	for rows.Next() {
		var item legacyTelegramTokens
		if err := rows.Scan(
			&item.id,
			&item.botToken,
			&item.botTokenSecretID,
			&item.inboundToken,
			&item.inboundTokenSecretID,
		); err != nil {
			rows.Close()
			return 0, 0, err
		}
		legacy = append(legacy, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	botTokenCount := 0
	inboundTokenCount := 0
	for _, item := range legacy {
		wroteBotToken := false
		if item.botToken != "" && item.botTokenSecretID == "" {
			item.botTokenSecretID, err = store.PutTx(
				ctx, tx, "telegram_notification_settings:"+item.id, "bot_token", item.botToken,
			)
			if err != nil {
				return botTokenCount, inboundTokenCount, err
			}
			wroteBotToken = true
		}
		wroteInboundToken := false
		if item.inboundToken != "" && item.inboundTokenSecretID == "" {
			item.inboundTokenSecretID, err = store.PutTx(
				ctx, tx, "telegram_notification_settings:"+item.id, "inbound_token", item.inboundToken,
			)
			if err != nil {
				return botTokenCount, inboundTokenCount, err
			}
			wroteInboundToken = true
		}

		if _, err := tx.Exec(ctx, `
			UPDATE telegram_notification_settings
			SET bot_token = CASE WHEN $2 <> '' THEN '' ELSE bot_token END,
			    bot_token_secret_id = CASE WHEN $2 <> '' THEN $2 ELSE bot_token_secret_id END,
			    inbound_token = CASE WHEN $3 <> '' THEN '' ELSE inbound_token END,
			    inbound_token_secret_id = CASE WHEN $3 <> '' THEN $3 ELSE inbound_token_secret_id END,
			    updated_at = now()
			WHERE id = $1
		`, item.id, item.botTokenSecretID, item.inboundTokenSecretID); err != nil {
			return botTokenCount, inboundTokenCount, err
		}
		if wroteBotToken {
			botTokenCount++
		}
		if wroteInboundToken {
			inboundTokenCount++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return botTokenCount, inboundTokenCount, err
	}
	return botTokenCount, inboundTokenCount, nil
}

func backfillWikiResourcePasswords(ctx context.Context, database credentialDB, store secretWriter) (int, error) {
	tx, err := database.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin wiki resource password backfill: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, password, password_secret_id
		FROM wiki_page_resources
		WHERE password <> ''
	`)
	if err != nil {
		return 0, fmt.Errorf("list legacy wiki resource passwords: %w", err)
	}

	type legacyWikiResource struct {
		id               string
		password         string
		passwordSecretID string
	}
	legacy := make([]legacyWikiResource, 0)
	for rows.Next() {
		var item legacyWikiResource
		if err := rows.Scan(&item.id, &item.password, &item.passwordSecretID); err != nil {
			rows.Close()
			return 0, err
		}
		legacy = append(legacy, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	count := 0
	for _, item := range legacy {
		if item.passwordSecretID == "" {
			item.passwordSecretID, err = store.PutTx(
				ctx, tx, "wiki_page_resources:"+item.id, "password", item.password,
			)
			if err != nil {
				return count, err
			}
			count++
		}
		if _, err := tx.Exec(ctx, `
			UPDATE wiki_page_resources
			SET password = '',
			    password_secret_id = $2,
			    updated_at = now()
			WHERE id = $1
		`, item.id, item.passwordSecretID); err != nil {
			return count, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return count, err
	}
	return count, nil
}
