package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/platform/secret"

	"github.com/jackc/pgx/v5"
)

type storeDB interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type secretStore interface {
	PutTx(context.Context, pgx.Tx, string, string, string) (string, error)
	DeleteTx(context.Context, pgx.Tx, string) error
}

type Store struct {
	db      storeDB
	secrets secretStore
}

type Value struct {
	Key       string     `json:"key"`
	Value     any        `json:"value"`
	Sensitive bool       `json:"sensitive"`
	Redacted  bool       `json:"redacted"`
	HasValue  bool       `json:"has_value"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func NewStore(database *db.Pool, secrets *secret.Store) *Store {
	var secretBackend secretStore
	if secrets != nil {
		secretBackend = secrets
	}
	return &Store{db: database, secrets: secretBackend}
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
	if schema.Sensitive && s.secrets == nil {
		return Value{}, errors.New("settings secret store is unavailable")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Value{}, err
	}
	defer tx.Rollback(ctx)

	currentSecretID, err := settingSecretID(ctx, tx, schema.Key)
	if err != nil {
		return Value{}, err
	}
	if schema.Sensitive {
		err = s.putSensitive(ctx, tx, schema, value, raw, currentSecretID)
	} else {
		err = s.putPlain(ctx, tx, schema, raw, currentSecretID)
	}
	if err != nil {
		return Value{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Value{}, err
	}
	return s.Get(ctx, schema)
}

func (s *Store) Delete(ctx context.Context, schema Schema) error {
	if s == nil || s.db == nil {
		return errors.New("settings store is unavailable")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	secretID, err := settingSecretID(ctx, tx, schema.Key)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM platform_settings WHERE key = $1`, schema.Key); err != nil {
		return err
	}
	if secretID != "" {
		if s.secrets == nil {
			return errors.New("settings secret store is unavailable")
		}
		if err := s.secrets.DeleteTx(ctx, tx, secretID); err != nil {
			return fmt.Errorf("delete platform setting secret %q: %w", schema.Key, err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) putPlain(
	ctx context.Context,
	tx pgx.Tx,
	schema Schema,
	raw json.RawMessage,
	currentSecretID string,
) error {
	if currentSecretID != "" {
		if s.secrets == nil {
			return errors.New("settings secret store is unavailable")
		}
		if err := s.secrets.DeleteTx(ctx, tx, currentSecretID); err != nil {
			return fmt.Errorf("delete stale platform setting secret %q: %w", schema.Key, err)
		}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO platform_settings (key, value, secret_id, updated_at)
		VALUES ($1, $2::jsonb, '', now())
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			secret_id = '',
			updated_at = now()
	`, schema.Key, string(raw))
	return err
}

func (s *Store) putSensitive(
	ctx context.Context,
	tx pgx.Tx,
	schema Schema,
	value any,
	raw json.RawMessage,
	currentSecretID string,
) error {
	secretID := ""
	if value != nil {
		plaintext := string(raw)
		if text, ok := value.(string); ok {
			plaintext = text
		}
		var err error
		secretID, err = s.secrets.PutTx(ctx, tx, "platform_settings", schema.Key, plaintext)
		if err != nil {
			return fmt.Errorf("store platform setting secret %q: %w", schema.Key, err)
		}
	}
	if currentSecretID != "" && currentSecretID != secretID {
		if err := s.secrets.DeleteTx(ctx, tx, currentSecretID); err != nil {
			return fmt.Errorf("delete replaced platform setting secret %q: %w", schema.Key, err)
		}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO platform_settings (key, value, secret_id, updated_at)
		VALUES ($1, 'null'::jsonb, $2, now())
		ON CONFLICT (key) DO UPDATE SET
			value = 'null'::jsonb,
			secret_id = EXCLUDED.secret_id,
			updated_at = now()
	`, schema.Key, secretID)
	return err
}

func settingSecretID(ctx context.Context, tx pgx.Tx, key string) (string, error) {
	var secretID string
	err := tx.QueryRow(ctx, `
		SELECT secret_id
		FROM platform_settings
		WHERE key = $1
		FOR UPDATE
	`, key).Scan(&secretID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return secretID, err
}
