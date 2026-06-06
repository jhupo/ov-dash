package notifications

import (
	"context"
	"errors"

	"ov-dash/backend/internal/db"

	"github.com/jackc/pgx/v5"
)

type Repository struct {
	db *db.Pool
}

func NewRepository(db *db.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetTelegramSettings(ctx context.Context) (TelegramSettings, error) {
	var settings TelegramSettings
	err := r.db.QueryRow(ctx, `
		INSERT INTO telegram_notification_settings (id)
		VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
		RETURNING id, enabled, bot_token, inbound_token, updated_at
	`, DefaultTelegramSettingsID).Scan(
		&settings.ID,
		&settings.Enabled,
		&settings.BotToken,
		&settings.InboundToken,
		&settings.UpdatedAt,
	)
	return settings, err
}

func (r *Repository) UpdateTelegramSettings(ctx context.Context, input UpdateTelegramSettingsInput) (TelegramSettings, error) {
	current, err := r.GetTelegramSettings(ctx)
	if err != nil {
		return TelegramSettings{}, err
	}

	botToken := current.BotToken
	if input.ClearBotToken {
		botToken = ""
	} else if input.BotToken != nil {
		botToken = *input.BotToken
	}

	inboundToken := current.InboundToken
	if input.ClearInboundToken {
		inboundToken = ""
	} else if input.InboundToken != nil {
		inboundToken = *input.InboundToken
	}

	var settings TelegramSettings
	err = r.db.QueryRow(ctx, `
		UPDATE telegram_notification_settings
		SET enabled = $2,
			bot_token = $3,
			inbound_token = $4,
			updated_at = now()
		WHERE id = $1
		RETURNING id, enabled, bot_token, inbound_token, updated_at
	`, DefaultTelegramSettingsID, input.Enabled, botToken, inboundToken).Scan(
		&settings.ID,
		&settings.Enabled,
		&settings.BotToken,
		&settings.InboundToken,
		&settings.UpdatedAt,
	)
	return settings, err
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
