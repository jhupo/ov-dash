package settings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSensitivePutRollsBackSecretWhenSettingWriteFails(t *testing.T) {
	writeErr := errors.New("setting write failed")
	tx := &settingsTestTx{
		row: settingsScanRow(func(dest ...any) error {
			*(dest[0].(*string)) = "sec_old"
			return nil
		}),
		execErr: writeErr,
	}
	secrets := &settingsTestSecrets{putID: "sec_new"}
	store := &Store{db: &settingsTestDB{tx: tx}, secrets: secrets}

	_, err := store.Put(context.Background(), sensitiveStringSchema(), json.RawMessage(`"next"`))
	if !errors.Is(err, writeErr) {
		t.Fatalf("Put error = %v, want %v", err, writeErr)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("transaction state committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
	if secrets.putTx != tx || len(secrets.deleteTxs) != 1 || secrets.deleteTxs[0] != tx {
		t.Fatal("secret operations did not use the settings transaction")
	}
}

func TestDeleteRollsBackSettingWhenSecretDeleteFails(t *testing.T) {
	deleteErr := errors.New("secret delete failed")
	tx := &settingsTestTx{row: settingsScanRow(func(dest ...any) error {
		*(dest[0].(*string)) = "sec_existing"
		return nil
	})}
	store := &Store{
		db:      &settingsTestDB{tx: tx},
		secrets: &settingsTestSecrets{deleteErr: deleteErr},
	}

	err := store.Delete(context.Background(), sensitiveStringSchema())
	if !errors.Is(err, deleteErr) {
		t.Fatalf("Delete error = %v, want %v", err, deleteErr)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("transaction state committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func sensitiveStringSchema() Schema {
	return Schema{Key: "integration.token", Type: TypeString, Sensitive: true, Writable: true}
}

type settingsTestDB struct {
	tx *settingsTestTx
}

func (db *settingsTestDB) Begin(context.Context) (pgx.Tx, error) {
	return db.tx, nil
}

func (*settingsTestDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return settingsScanRow(func(...any) error { return pgx.ErrNoRows })
}

type settingsTestTx struct {
	pgx.Tx
	row        pgx.Row
	execErr    error
	committed  bool
	rolledBack bool
}

func (tx *settingsTestTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return tx.row
}

func (tx *settingsTestTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), tx.execErr
}

func (tx *settingsTestTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *settingsTestTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

type settingsTestSecrets struct {
	putID     string
	putTx     pgx.Tx
	deleteTxs []pgx.Tx
	deleteErr error
}

func (s *settingsTestSecrets) PutTx(_ context.Context, tx pgx.Tx, _, _, _ string) (string, error) {
	s.putTx = tx
	return s.putID, nil
}

func (s *settingsTestSecrets) DeleteTx(_ context.Context, tx pgx.Tx, _ string) error {
	s.deleteTxs = append(s.deleteTxs, tx)
	return s.deleteErr
}

type settingsScanRow func(...any) error

func (row settingsScanRow) Scan(dest ...any) error {
	return row(dest...)
}
