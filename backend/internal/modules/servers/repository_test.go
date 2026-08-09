package servers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRepositoryGetDoesNotResolveCredentials(t *testing.T) {
	secrets := &serverTestSecrets{}
	repo := &Repository{
		db:      &serverTestDB{row: serverConnectionRow("sec_password", "sec_key")},
		secrets: secrets,
	}

	item, err := repo.Get(context.Background(), "server-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if item.Password != "" || item.PrivateKey != "" {
		t.Fatalf("inventory query exposed credentials: %#v", item)
	}
	if secrets.getCalls != 0 {
		t.Fatalf("inventory query resolved %d secrets, want 0", secrets.getCalls)
	}
}

func TestRepositoryGetWithCredentialsPropagatesDecryptionFailure(t *testing.T) {
	decryptErr := errors.New("unknown encryption key")
	repo := &Repository{
		db:      &serverTestDB{row: serverConnectionRow("sec_password", "")},
		secrets: &serverTestSecrets{getErr: decryptErr},
	}

	_, err := repo.GetWithCredentials(context.Background(), "server-1")
	if !errors.Is(err, decryptErr) || !strings.Contains(err.Error(), "decrypt server password secret") {
		t.Fatalf("GetWithCredentials error = %v, want explicit decryption error", err)
	}
}

func TestRepositoryUpsertRollsBackSecretWhenBusinessWriteFails(t *testing.T) {
	writeErr := errors.New("server upsert failed")
	tx := &serverTestTx{
		rows: []pgx.Row{
			serverScanRow(func(dest ...any) error {
				*(dest[0].(*string)) = "sec_old"
				*(dest[1].(*string)) = ""
				return nil
			}),
			serverScanRow(func(...any) error { return writeErr }),
		},
	}
	secrets := &serverTestSecrets{putID: "sec_new"}
	repo := &Repository{db: &serverTestDB{tx: tx}, secrets: secrets}
	password := "next-password"

	_, err := repo.Upsert(context.Background(), SaveInput{
		ID: "server-1", Name: "server", Host: "127.0.0.1", Port: 22,
		Username: "root", AuthType: "password", Password: &password,
	})
	if !errors.Is(err, writeErr) {
		t.Fatalf("Upsert error = %v, want %v", err, writeErr)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("transaction state committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
	if secrets.putTx != tx || len(secrets.deleteTxs) != 1 || secrets.deleteTxs[0] != tx {
		t.Fatalf("secret operations did not use the business transaction")
	}
	for _, arg := range tx.lastQueryArgs {
		if value, ok := arg.(string); ok && value == password {
			t.Fatal("plaintext password was passed to the server_connections upsert")
		}
	}
}

func TestRepositoryDeleteRollsBackWhenSecretDeleteFails(t *testing.T) {
	deleteErr := errors.New("secret delete failed")
	tx := &serverTestTx{rows: []pgx.Row{serverScanRow(func(dest ...any) error {
		*(dest[0].(*string)) = "sec_password"
		*(dest[1].(*string)) = ""
		return nil
	})}}
	repo := &Repository{
		db:      &serverTestDB{tx: tx},
		secrets: &serverTestSecrets{deleteErr: deleteErr},
	}

	err := repo.Delete(context.Background(), "server-1")
	if !errors.Is(err, deleteErr) {
		t.Fatalf("Delete error = %v, want %v", err, deleteErr)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("transaction state committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

type serverTestDB struct {
	tx  *serverTestTx
	row pgx.Row
}

func (db *serverTestDB) Begin(context.Context) (pgx.Tx, error) {
	return db.tx, nil
}

func (db *serverTestDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("INSERT 1"), nil
}

func (db *serverTestDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (db *serverTestDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return db.row
}

type serverTestTx struct {
	pgx.Tx
	rows          []pgx.Row
	rowIndex      int
	lastQueryArgs []any
	committed     bool
	rolledBack    bool
}

func (tx *serverTestTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("DELETE 1"), nil
}

func (tx *serverTestTx) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	tx.lastQueryArgs = args
	row := tx.rows[tx.rowIndex]
	tx.rowIndex++
	return row
}

func (tx *serverTestTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *serverTestTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

type serverTestSecrets struct {
	putID     string
	putTx     pgx.Tx
	deleteTxs []pgx.Tx
	deleteErr error
	getErr    error
	getCalls  int
}

func (s *serverTestSecrets) PutTx(_ context.Context, tx pgx.Tx, _, _, _ string) (string, error) {
	s.putTx = tx
	return s.putID, nil
}

func (s *serverTestSecrets) Get(context.Context, string) (string, error) {
	s.getCalls++
	return "", s.getErr
}

func (s *serverTestSecrets) DeleteTx(_ context.Context, tx pgx.Tx, _ string) error {
	s.deleteTxs = append(s.deleteTxs, tx)
	return s.deleteErr
}

type serverScanRow func(...any) error

func (row serverScanRow) Scan(dest ...any) error {
	return row(dest...)
}

func serverConnectionRow(passwordSecretID string, privateKeySecretID string) pgx.Row {
	return serverScanRow(func(dest ...any) error {
		*(dest[0].(*string)) = "server-1"
		*(dest[1].(*string)) = "server"
		*(dest[2].(*string)) = "default"
		*(dest[3].(*string)) = "local"
		*(dest[4].(*string)) = "127.0.0.1"
		*(dest[5].(*int)) = 22
		*(dest[6].(*string)) = "root"
		*(dest[7].(*string)) = "password"
		*(dest[8].(*string)) = passwordSecretID
		*(dest[9].(*string)) = privateKeySecretID
		*(dest[10].(**time.Time)) = nil
		*(dest[11].(*time.Time)) = time.Unix(1, 0)
		*(dest[12].(*time.Time)) = time.Unix(1, 0)
		return nil
	})
}
