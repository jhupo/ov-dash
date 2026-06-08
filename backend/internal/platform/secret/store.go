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
	"io"
	"strings"

	"ov-dash/backend/internal/db"
)

var ErrNotFound = errors.New("secret not found")

type Store struct {
	db  *db.Pool
	aes cipher.AEAD
}

func NewStore(db *db.Pool, key string) *Store {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		panic(err)
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return &Store{db: db, aes: aesgcm}
}

func (s *Store) Put(ctx context.Context, scope string, name string, plaintext string) (string, error) {
	if s == nil {
		return "", errors.New("secret store is nil")
	}
	id := secretID(scope, name)
	nonce := make([]byte, s.aes.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := s.aes.Seal(nil, nonce, []byte(plaintext), []byte(id))
	_, err := s.db.Exec(ctx, `
		INSERT INTO secrets (id, scope, name, nonce, ciphertext, key_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'local', now())
		ON CONFLICT (id) DO UPDATE SET
			scope = EXCLUDED.scope,
			name = EXCLUDED.name,
			nonce = EXCLUDED.nonce,
			ciphertext = EXCLUDED.ciphertext,
			key_id = EXCLUDED.key_id,
			updated_at = now()
	`, id, normalizePart(scope), normalizePart(name), encode(nonce), encode(ciphertext))
	return id, err
}

func (s *Store) Get(ctx context.Context, id string) (string, error) {
	if s == nil {
		return "", ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", ErrNotFound
	}

	var nonceRaw string
	var ciphertextRaw string
	err := s.db.QueryRow(ctx, `
		SELECT nonce, ciphertext
		FROM secrets
		WHERE id = $1
	`, id).Scan(&nonceRaw, &ciphertextRaw)
	if err != nil {
		return "", ErrNotFound
	}

	nonce, err := decode(nonceRaw)
	if err != nil {
		return "", err
	}
	ciphertext, err := decode(ciphertextRaw)
	if err != nil {
		return "", err
	}
	plaintext, err := s.aes.Open(nil, nonce, ciphertext, []byte(id))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if s == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `DELETE FROM secrets WHERE id = $1`, strings.TrimSpace(id))
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
