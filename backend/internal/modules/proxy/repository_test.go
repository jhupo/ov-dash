package proxy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRepositoryGetPropagatesSecretDecryptionFailure(t *testing.T) {
	decryptErr := errors.New("ciphertext rejected")
	repo := &Repository{
		db: &proxyTestDB{row: proxyScanRow(func(dest ...any) error {
			setProxySettingsRow(dest, "sec_password")
			return nil
		})},
		secrets: &proxyTestSecrets{getErr: decryptErr},
	}

	_, err := repo.Get(context.Background())
	if !errors.Is(err, decryptErr) || !strings.Contains(err.Error(), "decrypt proxy password secret") {
		t.Fatalf("Get error = %v, want explicit decryption error", err)
	}
}

func TestRepositoryUpdateRollsBackSecretChangesWhenBusinessWriteFails(t *testing.T) {
	writeErr := errors.New("proxy update failed")
	tx := &proxyTestTx{
		rows: []pgx.Row{
			proxyScanRow(func(dest ...any) error {
				*(dest[0].(*string)) = "sec_old"
				return nil
			}),
			proxyScanRow(func(...any) error { return writeErr }),
		},
	}
	secrets := &proxyTestSecrets{putID: "sec_new"}
	repo := &Repository{db: &proxyTestDB{tx: tx}, secrets: secrets}
	password := "next-password"

	_, err := repo.Update(context.Background(), UpdateSettingsInput{
		Enabled: true, Host: "127.0.0.1", Port: 1080, Password: &password,
	})
	if !errors.Is(err, writeErr) {
		t.Fatalf("Update error = %v, want %v", err, writeErr)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("transaction state committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
	if secrets.putTx != tx || len(secrets.deleteTxs) != 1 || secrets.deleteTxs[0] != tx {
		t.Fatalf("secret operations did not use the business transaction")
	}
}

type proxyTestDB struct {
	tx  *proxyTestTx
	row pgx.Row
}

func (db *proxyTestDB) Begin(context.Context) (pgx.Tx, error) {
	return db.tx, nil
}

func (db *proxyTestDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return db.row
}

type proxyTestTx struct {
	pgx.Tx
	rows       []pgx.Row
	rowIndex   int
	committed  bool
	rolledBack bool
}

func (tx *proxyTestTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("INSERT 1"), nil
}

func (tx *proxyTestTx) QueryRow(context.Context, string, ...any) pgx.Row {
	row := tx.rows[tx.rowIndex]
	tx.rowIndex++
	return row
}

func (tx *proxyTestTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *proxyTestTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

type proxyTestSecrets struct {
	putID     string
	putTx     pgx.Tx
	deleteTxs []pgx.Tx
	getErr    error
	plaintext string
}

func (s *proxyTestSecrets) PutTx(_ context.Context, tx pgx.Tx, _, _, _ string) (string, error) {
	s.putTx = tx
	return s.putID, nil
}

func (s *proxyTestSecrets) Get(context.Context, string) (string, error) {
	return s.plaintext, s.getErr
}

func (s *proxyTestSecrets) GetTx(context.Context, pgx.Tx, string) (string, error) {
	return s.plaintext, s.getErr
}

func (s *proxyTestSecrets) DeleteTx(_ context.Context, tx pgx.Tx, _ string) error {
	s.deleteTxs = append(s.deleteTxs, tx)
	return nil
}

type proxyScanRow func(...any) error

func (row proxyScanRow) Scan(dest ...any) error {
	return row(dest...)
}

func setProxySettingsRow(dest []any, secretID string) {
	*(dest[0].(*string)) = DefaultSettingsID
	*(dest[1].(*bool)) = true
	*(dest[2].(*string)) = "socks5"
	*(dest[3].(*string)) = "127.0.0.1"
	*(dest[4].(*int)) = 1080
	*(dest[5].(*string)) = "user"
	*(dest[6].(*string)) = secretID
	*(dest[7].(*time.Time)) = time.Unix(1, 0)
}
