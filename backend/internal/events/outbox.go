package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ov-dash/backend/internal/platform/redact"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	defaultMaxAttempts = 10
	baseRetryBackoff   = time.Second
	maxRetryBackoff    = 5 * time.Minute
)

var ErrLeaseLost = errors.New("outbox lease lost")

type Execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type Transaction interface {
	Execer
	Commit(context.Context) error
	Rollback(context.Context) error
}

type OutboxDB interface {
	Execer
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Outbox struct {
	db OutboxDB
}

type OutboxRecord struct {
	Event
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	LeaseOwner     string     `json:"lease_owner"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at"`
}

func NewOutbox(db OutboxDB) *Outbox {
	return &Outbox{db: db}
}

// Append writes through the caller's transaction so domain state and its event commit together.
func (o *Outbox) Append(ctx context.Context, tx Transaction, event Event) error {
	if tx == nil {
		return errors.New("append outbox event: transaction is nil")
	}
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Type) == "" {
		return errors.New("append outbox event: event id and type are required")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	payload, err := encodeRedactedPayload(event.Payload)
	if err != nil {
		return fmt.Errorf("append outbox event: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO event_outbox (
			id, event_type, source, payload, created_at, available_at, max_attempts
		)
		VALUES ($1, $2, $3, $4::jsonb, $5, $5, $6)
	`, event.ID, event.Type, event.Source, payload, event.CreatedAt, defaultMaxAttempts)
	if err != nil {
		return fmt.Errorf("append outbox event: %w", err)
	}
	return nil
}

func encodeRedactedPayload(payload map[string]any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	normalized := map[string]any{}
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, fmt.Errorf("normalize payload: %w", err)
	}
	encoded, err := json.Marshal(redact.Metadata(normalized))
	if err != nil {
		return nil, fmt.Errorf("encode redacted payload: %w", err)
	}
	return encoded, nil
}

func (o *Outbox) Claim(ctx context.Context, owner string, limit int, lease time.Duration) ([]OutboxRecord, error) {
	owner = strings.TrimSpace(owner)
	if o == nil || o.db == nil {
		return nil, errors.New("claim outbox events: database is nil")
	}
	if owner == "" {
		return nil, errors.New("claim outbox events: lease owner is required")
	}
	if limit <= 0 {
		return nil, errors.New("claim outbox events: limit must be positive")
	}
	if lease <= 0 {
		return nil, errors.New("claim outbox events: lease must be positive")
	}
	if lease.Milliseconds() == 0 {
		return nil, errors.New("claim outbox events: lease must be at least one millisecond")
	}

	var encoded []byte
	err := o.db.QueryRow(ctx, `
		WITH candidates AS (
			SELECT id
			FROM event_outbox
			WHERE delivered_at IS NULL
			  AND attempts < max_attempts
			  AND available_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at <= now())
			ORDER BY available_at, created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		), claimed AS (
			UPDATE event_outbox AS outbox
			SET lease_owner = $1,
				lease_expires_at = now() + ($3::bigint * interval '1 millisecond'),
				attempts = outbox.attempts + 1,
				updated_at = now()
			FROM candidates
			WHERE outbox.id = candidates.id
			RETURNING outbox.id, outbox.event_type, outbox.source, outbox.payload,
				outbox.created_at, outbox.attempts, outbox.max_attempts,
				outbox.lease_owner, outbox.lease_expires_at
		)
		SELECT COALESCE(jsonb_agg(jsonb_build_object(
			'id', id,
			'type', event_type,
			'source', source,
			'payload', payload,
			'created_at', created_at,
			'attempts', attempts,
			'max_attempts', max_attempts,
			'lease_owner', lease_owner,
			'lease_expires_at', lease_expires_at
		) ORDER BY created_at, id), '[]'::jsonb)
		FROM claimed
	`, owner, limit, lease.Milliseconds()).Scan(&encoded)
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}

	var records []OutboxRecord
	if err := json.Unmarshal(encoded, &records); err != nil {
		return nil, fmt.Errorf("claim outbox events: decode rows: %w", err)
	}
	return records, nil
}

func (o *Outbox) Ack(ctx context.Context, id string, owner string) error {
	if o == nil || o.db == nil {
		return errors.New("ack outbox event: database is nil")
	}
	tag, err := o.db.Exec(ctx, `
		UPDATE event_outbox
		SET delivered_at = now(),
			lease_owner = '',
			lease_expires_at = NULL,
			last_error = '',
			updated_at = now()
		WHERE id = $1
		  AND delivered_at IS NULL
		  AND lease_owner = $2
		  AND lease_expires_at > now()
	`, id, owner)
	if err != nil {
		return fmt.Errorf("ack outbox event: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (o *Outbox) Fail(ctx context.Context, record OutboxRecord, owner string, cause error) error {
	if o == nil || o.db == nil {
		return errors.New("fail outbox event: database is nil")
	}
	message := "event dispatch failed"
	if cause != nil {
		message = redact.Text(cause.Error())
	}
	backoff := retryBackoff(record.Attempts)
	tag, err := o.db.Exec(ctx, `
		UPDATE event_outbox
		SET available_at = CASE
				WHEN attempts < max_attempts
				THEN now() + ($4::bigint * interval '1 millisecond')
				ELSE available_at
			END,
			lease_owner = '',
			lease_expires_at = NULL,
			last_error = $3,
			updated_at = now()
		WHERE id = $1
		  AND delivered_at IS NULL
		  AND lease_owner = $2
		  AND lease_expires_at > now()
	`, record.ID, owner, message, backoff.Milliseconds())
	if err != nil {
		return fmt.Errorf("fail outbox event: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}

func retryBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return baseRetryBackoff
	}
	backoff := baseRetryBackoff
	for current := 1; current < attempt && backoff < maxRetryBackoff; current++ {
		backoff *= 2
		if backoff >= maxRetryBackoff {
			return maxRetryBackoff
		}
	}
	return backoff
}
