package settings

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/platform/secret"

	"github.com/jackc/pgx/v5"
)

type Store struct {
	db      *db.Pool
	secrets *secret.Store
}

type Value struct {
	Key       string     `json:"key"`
	Value     any        `json:"value"`
	Sensitive bool       `json:"sensitive"`
	Redacted  bool       `json:"redacted"`
	HasValue  bool       `json:"has_value"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func NewStore(db *db.Pool, secrets *secret.Store) *Store {
	return &Store{db: db, secrets: secrets}
}

func (s *Store) Get(ctx context.Context, schema Schema) (Value, error) {
	item := Value{
		Key:       schema.Key,
		Value:     schema.Default,
		Sensitive: schema.Sensitive,
		Redacted:  false,
		HasValue:  false,
	}
	if schema.Sensitive {
		item.Value = nil
		item.Redacted = true
	}

	if s == nil || s.db == nil {
		return item, errors.New("settings store is unavailable")
	}

	var raw []byte
	var secretID string
	var updatedAt time.Time
	err := s.db.QueryRow(ctx, `
		SELECT value, secret_id, updated_at
		FROM platform_settings
		WHERE key = $1
	`, schema.Key).Scan(&raw, &secretID, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, nil
	}
	if err != nil {
		return item, err
	}

	item.UpdatedAt = &updatedAt
	if schema.Sensitive {
		item.HasValue = strings.TrimSpace(secretID) != ""
		return item, nil
	}

	value, err := Decode(json.RawMessage(raw))
	if err != nil {
		return item, err
	}
	item.Value = value
	item.HasValue = true
	return item, nil
}

func (s *Store) Put(ctx context.Context, schema Schema, raw json.RawMessage) (Value, error) {
	if s == nil || s.db == nil {
		return Value{}, errors.New("settings store is unavailable")
	}
	value, err := Validate(schema, raw)
	if err != nil {
		return Value{}, err
	}

	if schema.Sensitive {
		return s.putSensitive(ctx, schema, value, raw)
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO platform_settings (key, value, secret_id, updated_at)
		VALUES ($1, $2::jsonb, '', now())
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			secret_id = '',
			updated_at = now()
	`, schema.Key, string(raw)); err != nil {
		return Value{}, err
	}
	return s.Get(ctx, schema)
}

func (s *Store) putSensitive(ctx context.Context, schema Schema, value any, raw json.RawMessage) (Value, error) {
	if value == nil {
		if s.secrets != nil {
			_ = s.secrets.DeleteNamed(ctx, "platform_settings", schema.Key)
		}
		if _, err := s.db.Exec(ctx, `
			INSERT INTO platform_settings (key, value, secret_id, updated_at)
			VALUES ($1, 'null'::jsonb, '', now())
			ON CONFLICT (key) DO UPDATE SET
				value = 'null'::jsonb,
				secret_id = '',
				updated_at = now()
		`, schema.Key); err != nil {
			return Value{}, err
		}
		return s.Get(ctx, schema)
	}

	if s.secrets == nil {
		return Value{}, errors.New("secret store is unavailable")
	}
	plaintext := string(raw)
	if text, ok := value.(string); ok {
		plaintext = text
	}
	secretID, err := s.secrets.Put(ctx, "platform_settings", schema.Key, plaintext)
	if err != nil {
		return Value{}, err
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO platform_settings (key, value, secret_id, updated_at)
		VALUES ($1, 'null'::jsonb, $2, now())
		ON CONFLICT (key) DO UPDATE SET
			value = 'null'::jsonb,
			secret_id = EXCLUDED.secret_id,
			updated_at = now()
	`, schema.Key, secretID); err != nil {
		return Value{}, err
	}
	return s.Get(ctx, schema)
}
