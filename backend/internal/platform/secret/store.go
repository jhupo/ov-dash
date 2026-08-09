package secret

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"ov-dash/backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrNotFound = errors.New("secret not found")
var ErrUnknownKey = errors.New("secret encryption key is not configured")

type Store struct {
	db          *db.Pool
	activeKeyID string
	keys        map[string]cipher.AEAD
}

func NewStore(db *db.Pool, activeKeyID string, keyring map[string]string) (*Store, error) {
	activeKeyID = strings.TrimSpace(activeKeyID)
	if db == nil || activeKeyID == "" {
		return nil, errors.New("secret database and active key id are required")
	}
	keys := make(map[string]cipher.AEAD, len(keyring))
	for id, key := range keyring {
		id = strings.TrimSpace(id)
		key = strings.TrimSpace(key)
		if id == "" || key == "" {
			return nil, errors.New("secret key id and value are required")
		}
		sum := sha256.Sum256([]byte(key))
		block, err := aes.NewCipher(sum[:])
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		keys[id] = aead
	}
	if _, exists := keys[activeKeyID]; !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnknownKey, activeKeyID)
	}
	return &Store{db: db, activeKeyID: activeKeyID, keys: keys}, nil
}

func (s *Store) Put(ctx context.Context, scope string, name string, plaintext string) (string, error) {
	if s == nil {
		return "", errors.New("secret store is nil")
	}
	return s.put(ctx, s.db, scope, name, plaintext)
}

func (s *Store) PutTx(ctx context.Context, tx pgx.Tx, scope string, name string, plaintext string) (string, error) {
	if tx == nil {
		return "", errors.New("secret transaction is nil")
	}
	return s.put(ctx, tx, scope, name, plaintext)
}

func (s *Store) put(ctx context.Context, executor secretExecer, scope string, name string, plaintext string) (string, error) {
	if s == nil {
		return "", errors.New("secret store is nil")
	}
	id := secretID(scope, name)
	nonce, ciphertext, err := s.encrypt(id, plaintext)
	if err != nil {
		return "", err
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO secrets (id, scope, name, nonce, ciphertext, key_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (id) DO UPDATE SET
			scope = EXCLUDED.scope,
			name = EXCLUDED.name,
			nonce = EXCLUDED.nonce,
			ciphertext = EXCLUDED.ciphertext,
			key_id = EXCLUDED.key_id,
			updated_at = now()
	`, id, normalizePart(scope), normalizePart(name), encode(nonce), encode(ciphertext), s.activeKeyID)
	return id, err
}

func (s *Store) Get(ctx context.Context, id string) (string, error) {
	if s == nil {
		return "", ErrNotFound
	}
	return s.get(ctx, s.db, id)
}

func (s *Store) GetTx(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	if tx == nil {
		return "", errors.New("secret transaction is nil")
	}
	return s.get(ctx, tx, id)
}

func (s *Store) get(ctx context.Context, queryer secretQueryer, id string) (string, error) {
	if s == nil {
		return "", ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", ErrNotFound
	}

	var nonceRaw string
	var ciphertextRaw string
	var keyID string
	err := queryer.QueryRow(ctx, `
		SELECT nonce, ciphertext, key_id
		FROM secrets
		WHERE id = $1
	`, id).Scan(&nonceRaw, &ciphertextRaw, &keyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read secret %s: %w", id, err)
	}

	nonce, err := decode(nonceRaw)
	if err != nil {
		return "", err
	}
	ciphertext, err := decode(ciphertextRaw)
	if err != nil {
		return "", err
	}
	aead, exists := s.keys[keyID]
	if !exists {
		return "", fmt.Errorf("%w: %s", ErrUnknownKey, keyID)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(id))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *Store) Rotate(ctx context.Context) (int, error) {
	if s == nil {
		return 0, errors.New("secret store is nil")
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, nonce, ciphertext, key_id
		FROM secrets
		WHERE key_id <> $1
		ORDER BY id
	`, s.activeKeyID)
	if err != nil {
		return 0, fmt.Errorf("list secrets for rotation: %w", err)
	}
	defer rows.Close()

	rotated := 0
	for rows.Next() {
		var id, nonceRaw, ciphertextRaw, keyID string
		if err := rows.Scan(&id, &nonceRaw, &ciphertextRaw, &keyID); err != nil {
			return rotated, err
		}
		aead, exists := s.keys[keyID]
		if !exists {
			return rotated, fmt.Errorf("%w: %s", ErrUnknownKey, keyID)
		}
		nonce, err := decode(nonceRaw)
		if err != nil {
			return rotated, fmt.Errorf("decode secret %s nonce: %w", id, err)
		}
		ciphertext, err := decode(ciphertextRaw)
		if err != nil {
			return rotated, fmt.Errorf("decode secret %s ciphertext: %w", id, err)
		}
		plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(id))
		if err != nil {
			return rotated, fmt.Errorf("decrypt secret %s: %w", id, err)
		}
		newNonce, newCiphertext, err := s.encrypt(id, string(plaintext))
		if err != nil {
			return rotated, err
		}
		tag, err := s.db.Exec(ctx, `
			UPDATE secrets
			SET nonce = $2, ciphertext = $3, key_id = $4, updated_at = now()
			WHERE id = $1 AND key_id = $5
		`, id, encode(newNonce), encode(newCiphertext), s.activeKeyID, keyID)
		if err != nil {
			return rotated, fmt.Errorf("rotate secret %s: %w", id, err)
		}
		if tag.RowsAffected() == 1 {
			rotated++
		}
	}
	return rotated, rows.Err()
}

func (s *Store) encrypt(id string, plaintext string) ([]byte, []byte, error) {
	aead, exists := s.keys[s.activeKeyID]
	if !exists {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnknownKey, s.activeKeyID)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(nil, nonce, []byte(plaintext), []byte(id)), nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if s == nil {
		return nil
	}
	return s.delete(ctx, s.db, id)
}

func (s *Store) DeleteTx(ctx context.Context, tx pgx.Tx, id string) error {
	if tx == nil {
		return errors.New("secret transaction is nil")
	}
	return s.delete(ctx, tx, id)
}

func (s *Store) delete(ctx context.Context, executor secretExecer, id string) error {
	if s == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	_, err := executor.Exec(ctx, `DELETE FROM secrets WHERE id = $1`, strings.TrimSpace(id))
	return err
}

func (s *Store) DeleteNamed(ctx context.Context, scope string, name string) error {
	return s.Delete(ctx, secretID(scope, name))
}

func secretID(scope string, name string) string {
	sum := sha256.Sum256([]byte(normalizePart(scope) + ":" + normalizePart(name)))
	return "sec_" + hex.EncodeToString(sum[:16])
}

func normalizePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	return value
}

func encode(value []byte) string {
	return base64.RawStdEncoding.EncodeToString(value)
}

func decode(value string) ([]byte, error) {
	return base64.RawStdEncoding.DecodeString(value)
}

type secretExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type secretQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}
