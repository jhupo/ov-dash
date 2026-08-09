package notifications

import (
	"context"
	"errors"

	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/platform/secret"

	"github.com/jackc/pgx/v5"
)

type Repository struct {
	db      *db.Pool
	secrets *secret.Store
}

func NewRepository(db *db.Pool, secrets *secret.Store) *Repository {
	return &Repository{db: db, secrets: secrets}
}

func (r *Repository) GetTelegramSettings(ctx context.Context) (TelegramSettings, error) {
	var settings TelegramSettings
	err := r.db.QueryRow(ctx, `
		INSERT INTO telegram_notification_settings (id)
		VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
		RETURNING id, enabled, bot_token_secret_id, inbound_token_secret_id, group_enabled, group_chat_id, updated_at
	`, DefaultTelegramSettingsID).Scan(
		&settings.ID,
		&settings.Enabled,
		&settings.BotTokenSecretID,
		&settings.InboundTokenSecretID,
		&settings.GroupEnabled,
		&settings.GroupChatID,
		&settings.UpdatedAt,
	)
	if err != nil {
		return TelegramSettings{}, err
	}
	if err := r.resolveTelegramSecrets(ctx, &settings); err != nil {
		return TelegramSettings{}, err
	}
	return settings, nil
}

func (r *Repository) UpdateTelegramSettings(ctx context.Context, input UpdateTelegramSettingsInput) (TelegramSettings, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return TelegramSettings{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO telegram_notification_settings (id)
		VALUES ($1)
		ON CONFLICT (id) DO NOTHING
	`, DefaultTelegramSettingsID); err != nil {
		return TelegramSettings{}, err
	}

	var current TelegramSettings
	if err := tx.QueryRow(ctx, `
		SELECT id, enabled, bot_token_secret_id, inbound_token_secret_id, group_enabled, group_chat_id, updated_at
		FROM telegram_notification_settings
		WHERE id = $1
		FOR UPDATE
	`, DefaultTelegramSettingsID).Scan(
		&current.ID,
		&current.Enabled,
		&current.BotTokenSecretID,
		&current.InboundTokenSecretID,
		&current.GroupEnabled,
		&current.GroupChatID,
		&current.UpdatedAt,
	); err != nil {
		return TelegramSettings{}, err
	}
	if err := r.resolveTelegramSecretsTx(ctx, tx, &current); err != nil {
		return TelegramSettings{}, err
	}

	botToken, botTokenSecretID, err := r.updateTelegramSecret(
		ctx, tx, "bot_token", current.BotToken, current.BotTokenSecretID, input.BotToken, input.ClearBotToken,
	)
	if err != nil {
		return TelegramSettings{}, err
	}
	inboundToken, inboundTokenSecretID, err := r.updateTelegramSecret(
		ctx, tx, "inbound_token", current.InboundToken, current.InboundTokenSecretID, input.InboundToken, input.ClearInboundToken,
	)
	if err != nil {
		return TelegramSettings{}, err
	}

	var settings TelegramSettings
	err = tx.QueryRow(ctx, `
		UPDATE telegram_notification_settings
		SET enabled = $2,
			bot_token = '',
			bot_token_secret_id = $3,
			inbound_token = '',
			inbound_token_secret_id = $4,
			group_enabled = $5,
			group_chat_id = $6,
			updated_at = now()
		WHERE id = $1
		RETURNING id, enabled, bot_token_secret_id, inbound_token_secret_id, group_enabled, group_chat_id, updated_at
	`, DefaultTelegramSettingsID, input.Enabled, botTokenSecretID, inboundTokenSecretID, input.GroupEnabled, input.GroupChatID).Scan(
		&settings.ID,
		&settings.Enabled,
		&settings.BotTokenSecretID,
		&settings.InboundTokenSecretID,
		&settings.GroupEnabled,
		&settings.GroupChatID,
		&settings.UpdatedAt,
	)
	if err != nil {
		return TelegramSettings{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TelegramSettings{}, err
	}
	settings.BotToken = botToken
	settings.InboundToken = inboundToken
	return settings, nil
}

func (r *Repository) resolveTelegramSecrets(ctx context.Context, settings *TelegramSettings) error {
	var err error
	if settings.BotTokenSecretID != "" {
		settings.BotToken, err = r.secrets.Get(ctx, settings.BotTokenSecretID)
		if err != nil {
			return err
		}
	}
	if settings.InboundTokenSecretID != "" {
		settings.InboundToken, err = r.secrets.Get(ctx, settings.InboundTokenSecretID)
	}
	return err
}

func (r *Repository) resolveTelegramSecretsTx(ctx context.Context, tx pgx.Tx, settings *TelegramSettings) error {
	var err error
	if settings.BotTokenSecretID != "" {
		settings.BotToken, err = r.secrets.GetTx(ctx, tx, settings.BotTokenSecretID)
		if err != nil {
			return err
		}
	}
	if settings.InboundTokenSecretID != "" {
		settings.InboundToken, err = r.secrets.GetTx(ctx, tx, settings.InboundTokenSecretID)
	}
	return err
}

func (r *Repository) updateTelegramSecret(
	ctx context.Context,
	tx pgx.Tx,
	name string,
	currentValue string,
	currentSecretID string,
	nextValue *string,
	clear bool,
) (string, string, error) {
	if !clear && nextValue == nil {
		return currentValue, currentSecretID, nil
	}
	if clear || *nextValue == "" {
		if err := r.secrets.DeleteTx(ctx, tx, currentSecretID); err != nil {
			return "", "", err
		}
		return "", "", nil
	}
	secretID, err := r.secrets.PutTx(
		ctx,
		tx,
		"telegram_notification_settings:"+DefaultTelegramSettingsID,
		name,
		*nextValue,
	)
	if err != nil {
		return "", "", err
	}
	if currentSecretID != "" && currentSecretID != secretID {
		if err := r.secrets.DeleteTx(ctx, tx, currentSecretID); err != nil {
			return "", "", err
		}
	}
	return *nextValue, secretID, nil
}

func (r *Repository) ListUserTelegramSettings(ctx context.Context) ([]UserTelegramSettings, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id,
		       u.username,
		       u.email,
		       trim(concat(u.first_name, ' ', u.last_name)) AS full_name,
		       COALESCE(t.enabled, false) AS enabled,
		       COALESCE(t.chat_id, '') AS chat_id,
		       COALESCE(t.updated_at, u.updated_at) AS updated_at
		FROM users u
		LEFT JOIN user_telegram_settings t ON t.user_id = u.id
		ORDER BY u.username ASC, u.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]UserTelegramSettings, 0)
	for rows.Next() {
		var item UserTelegramSettings
		if err := rows.Scan(
			&item.UserID,
			&item.Username,
			&item.Email,
			&item.FullName,
			&item.Enabled,
			&item.ChatID,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) UpdateUserTelegramSettings(ctx context.Context, input UpdateUserTelegramSettingsInput) (UserTelegramSettings, error) {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_telegram_settings (user_id, enabled, chat_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			chat_id = EXCLUDED.chat_id,
			updated_at = now()
	`, input.UserID, input.Enabled, input.ChatID)
	if err != nil {
		return UserTelegramSettings{}, err
	}
	return r.GetUserTelegramSettings(ctx, input.UserID)
}

func (r *Repository) GetUserTelegramSettings(ctx context.Context, userID string) (UserTelegramSettings, error) {
	row := r.db.QueryRow(ctx, `
		SELECT u.id,
		       u.username,
		       u.email,
		       trim(concat(u.first_name, ' ', u.last_name)) AS full_name,
		       COALESCE(t.enabled, false) AS enabled,
		       COALESCE(t.chat_id, '') AS chat_id,
		       COALESCE(t.updated_at, u.updated_at) AS updated_at
		FROM users u
		LEFT JOIN user_telegram_settings t ON t.user_id = u.id
		WHERE u.id = $1
	`, userID)
	return scanUserTelegramSettings(row)
}

func (r *Repository) FindRecipient(ctx context.Context, userID string, username string) (UserTelegramSettings, error) {
	if userID != "" {
		return r.GetUserTelegramSettings(ctx, userID)
	}

	row := r.db.QueryRow(ctx, `
		SELECT u.id,
		       u.username,
		       u.email,
		       trim(concat(u.first_name, ' ', u.last_name)) AS full_name,
		       COALESCE(t.enabled, false) AS enabled,
		       COALESCE(t.chat_id, '') AS chat_id,
		       COALESCE(t.updated_at, u.updated_at) AS updated_at
		FROM users u
		LEFT JOIN user_telegram_settings t ON t.user_id = u.id
		WHERE lower(u.username) = lower($1)
	`, username)
	return scanUserTelegramSettings(row)
}

func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUserTelegramSettings(row rowScanner) (UserTelegramSettings, error) {
	var item UserTelegramSettings
	err := row.Scan(
		&item.UserID,
		&item.Username,
		&item.Email,
		&item.FullName,
		&item.Enabled,
		&item.ChatID,
		&item.UpdatedAt,
	)
	return item, err
}
