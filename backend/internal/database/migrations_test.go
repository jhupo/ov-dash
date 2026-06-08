package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestApplyFileRejectsAppliedMigrationChecksumMismatch(t *testing.T) {
	file := writeMigrationFile(t, "0019_private_platform_foundation.sql", "SELECT 1;")
	runner := &MigrationRunner{db: &fakeMigrationDB{
		existingChecksum: "different",
		existingStatus:   "applied",
	}}

	result, err := runner.ApplyFile(context.Background(), file)

	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch error, got %v", err)
	}
	if result.Status != "" {
		t.Fatalf("Status = %q, want empty because migration was not applied", result.Status)
	}
}

func TestApplyFileRejectsExistingFailedStatus(t *testing.T) {
	file := writeMigrationFile(t, "0019_private_platform_foundation.sql", "SELECT 1;")
	runner := &MigrationRunner{db: &fakeMigrationDB{
		existingChecksum: checksum([]byte("SELECT 1;")),
		existingStatus:   "failed",
	}}

	_, err := runner.ApplyFile(context.Background(), file)

	if err == nil || !strings.Contains(err.Error(), "has status failed") {
		t.Fatalf("expected failed status error, got %v", err)
	}
}

func TestApplyFileRecordsFailedMigrationWhenExecFails(t *testing.T) {
	applyErr := errors.New("boom")
	file := writeMigrationFile(t, "0020_job_infra_hardening.sql", "SELECT fail;")
	db := &fakeMigrationDB{execErr: applyErr}
	runner := &MigrationRunner{db: db}

	result, err := runner.ApplyFile(context.Background(), file)

	if err == nil || !strings.Contains(err.Error(), "apply migration") {
		t.Fatalf("expected apply migration error, got %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("Status = %q, want failed", result.Status)
	}
	if result.Error != applyErr.Error() {
		t.Fatalf("Error = %q, want %q", result.Error, applyErr.Error())
	}
	if db.failedRecord == nil {
		t.Fatal("expected failed migration to be recorded")
	}
	if db.failedRecord.version != "0020_job_infra_hardening" {
		t.Fatalf("failed version = %q", db.failedRecord.version)
	}
	if db.failedRecord.module != "jobs" {
		t.Fatalf("failed module = %q, want jobs", db.failedRecord.module)
	}
	if db.failedRecord.status != "failed" {
		t.Fatalf("failed status = %q, want failed", db.failedRecord.status)
	}
	if db.failedRecord.checksum != checksum([]byte("SELECT fail;")) {
		t.Fatalf("failed checksum = %q, want current file checksum", db.failedRecord.checksum)
	}
}

func TestMigrationVersionDerivesModule(t *testing.T) {
	tests := []struct {
		file       string
		wantVer    string
		wantModule string
	}{
		{"0013_job_platform_audit.sql", "0013_job_platform_audit", "jobs"},
		{"0012_auth_sessions.sql", "0012_auth_sessions", "auth"},
		{"0019_private_platform_foundation.sql", "0019_private_platform_foundation", "private"},
		{"custom.sql", "custom", "core"},
	}

	for _, tt := range tests {
		gotVer, gotModule := migrationVersion(tt.file)
		if gotVer != tt.wantVer || gotModule != tt.wantModule {
			t.Fatalf("migrationVersion(%q) = (%q, %q), want (%q, %q)", tt.file, gotVer, gotModule, tt.wantVer, tt.wantModule)
		}
	}
}

func TestDiagnoseMigrationFilesReportsDuplicateNumericPrefixes(t *testing.T) {
	files := []string{
		filepath.Join("migrations", "0010_first.sql"),
		filepath.Join("migrations", "0010_second.sql"),
		filepath.Join("migrations", "0011_next.sql"),
	}

	diagnostics := diagnoseMigrationFiles(files)

	if len(diagnostics) != 1 {
		t.Fatalf("len(diagnostics) = %d, want 1: %#v", len(diagnostics), diagnostics)
	}
	if diagnostics[0].Code != "duplicate_numeric_prefix" {
		t.Fatalf("code = %q, want duplicate_numeric_prefix", diagnostics[0].Code)
	}
	if !strings.Contains(diagnostics[0].Message, "0010") {
		t.Fatalf("message = %q, want numeric prefix", diagnostics[0].Message)
	}
	wantFiles := []string{"0010_first.sql", "0010_second.sql"}
	if strings.Join(diagnostics[0].Files, ",") != strings.Join(wantFiles, ",") {
		t.Fatalf("files = %#v, want %#v", diagnostics[0].Files, wantFiles)
	}
}

func TestMigrationDirectoryDoesNotAddUnexpectedDuplicateNumericPrefixes(t *testing.T) {
	files, _, err := migrationFiles(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("migrationFiles returned error: %v", err)
	}

	duplicates := duplicateMigrationNumericPrefixes(files)
	allowed := []string{"0013", "0021"}
	for prefix := range duplicates {
		if !slices.Contains(allowed, prefix) {
			t.Fatalf("unexpected duplicate migration numeric prefix %s: %#v", prefix, duplicates[prefix])
		}
	}
	for _, prefix := range allowed {
		if len(duplicates[prefix]) == 0 {
			t.Fatalf("allowed duplicate prefix %s is no longer present; remove it from the allowlist", prefix)
		}
	}
}

func TestWithMigrationLockConnLocksAroundApplyAndUnlocksOnError(t *testing.T) {
	conn := &fakeMigrationLockConn{}
	applyErr := errors.New("apply failed")

	err := withMigrationLockConn(context.Background(), conn, func(context.Context) error {
		conn.calls = append(conn.calls, "apply")
		return applyErr
	})

	if !errors.Is(err, applyErr) {
		t.Fatalf("error = %v, want %v", err, applyErr)
	}
	want := []string{"lock", "apply", "unlock"}
	if strings.Join(conn.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %#v, want %#v", conn.calls, want)
	}
	if len(conn.keys) != 2 {
		t.Fatalf("lock key calls = %#v, want two keys", conn.keys)
	}
	if conn.keys[0] != migrationLockKey || conn.keys[1] != migrationLockKey {
		t.Fatalf("keys = %#v, want migration lock key %d", conn.keys, migrationLockKey)
	}
}

func writeMigrationFile(t *testing.T, name string, contents string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
		t.Fatalf("write migration file: %v", err)
	}
	return file
}

type fakeMigrationDB struct {
	existingChecksum string
	existingStatus   string
	execErr          error
	failedRecord     *fakeFailedMigrationRecord
}

type fakeFailedMigrationRecord struct {
	version  string
	module   string
	name     string
	checksum string
	status   string
}

func (db *fakeMigrationDB) Begin(context.Context) (pgx.Tx, error) {
	return &fakeMigrationTx{db: db}, nil
}

func (db *fakeMigrationDB) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "'failed'") && len(arguments) >= 4 {
		db.failedRecord = &fakeFailedMigrationRecord{
			version:  arguments[0].(string),
			module:   arguments[1].(string),
			name:     arguments[2].(string),
			checksum: arguments[3].(string),
			status:   "failed",
		}
	}
	return pgconn.NewCommandTag("INSERT 1"), nil
}

func (db *fakeMigrationDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

type fakeMigrationTx struct {
	db *fakeMigrationDB
}

func (tx *fakeMigrationTx) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}

func (tx *fakeMigrationTx) Commit(context.Context) error {
	return nil
}

func (tx *fakeMigrationTx) Rollback(context.Context) error {
	return nil
}

func (tx *fakeMigrationTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}

func (tx *fakeMigrationTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

func (tx *fakeMigrationTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (tx *fakeMigrationTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}

func (tx *fakeMigrationTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "schema_migrations") {
		return pgconn.NewCommandTag("INSERT 1"), nil
	}
	if tx.db.execErr != nil {
		return pgconn.CommandTag{}, tx.db.execErr
	}
	return pgconn.NewCommandTag("SELECT 1"), nil
}

func (tx *fakeMigrationTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (tx *fakeMigrationTx) QueryRow(context.Context, string, ...any) pgx.Row {
	if tx.db.existingStatus == "" {
		return fakeMigrationRow{err: pgx.ErrNoRows}
	}
	return fakeMigrationRow{values: []string{tx.db.existingChecksum, tx.db.existingStatus}}
}

func (tx *fakeMigrationTx) Conn() *pgx.Conn {
	return nil
}

type fakeMigrationRow struct {
	values []string
	err    error
}

func (r fakeMigrationRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, value := range r.values {
		*(dest[i].(*string)) = value
	}
	return nil
}

type fakeMigrationLockConn struct {
	calls []string
	keys  []int64
}

func (c *fakeMigrationLockConn) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "pg_advisory_lock") {
		c.calls = append(c.calls, "lock")
	}
	if strings.Contains(sql, "pg_advisory_unlock") {
		c.calls = append(c.calls, "unlock")
	}
	if len(arguments) == 1 {
		c.keys = append(c.keys, arguments[0].(int64))
	}
	return pgconn.NewCommandTag("SELECT 1"), nil
}

func (c *fakeMigrationLockConn) Release() {}
