package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"ov-dash/backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MigrationRunner struct {
	db migrationDB
}

type migrationDB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type migrationLockDB interface {
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

type migrationLockConn interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Release()
}

const migrationLockKey int64 = 7181306580204105728

type MigrationSummary struct {
	Applied     []MigrationResult
	Skipped     []MigrationResult
	Failed      []MigrationResult
	Diagnostics []MigrationDiagnostic
	Duration    time.Duration
}

type MigrationResult struct {
	Version     string
	Module      string
	Name        string
	Status      string
	ExecutionMS int
	Error       string
}

type MigrationDiagnostic struct {
	Code    string
	Message string
	Files   []string
}

func NewMigrationRunner(db *db.Pool) *MigrationRunner {
	return &MigrationRunner{db: db}
}

func (r *MigrationRunner) ApplyDir(ctx context.Context, dir string) (MigrationSummary, error) {
	started := time.Now()
	var summary MigrationSummary

	err := r.withMigrationLock(ctx, func(ctx context.Context) error {
		if err := r.ensureSchemaMigrations(ctx); err != nil {
			return err
		}
		if err := r.ensureNoFailedMigrations(ctx); err != nil {
			return err
		}

		files, diagnostics, err := migrationFiles(dir)
		if err != nil {
			return err
		}
		summary.Diagnostics = append(summary.Diagnostics, diagnostics...)

		for _, file := range files {
			result, err := r.ApplyFile(ctx, file)
			switch result.Status {
			case "applied":
				summary.Applied = append(summary.Applied, result)
			case "skipped":
				summary.Skipped = append(summary.Skipped, result)
			case "failed":
				summary.Failed = append(summary.Failed, result)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	summary.Duration = time.Since(started)
	if err != nil {
		return summary, err
	}
	return summary, nil
}

func migrationFiles(dir string) ([]string, []MigrationDiagnostic, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	slices.Sort(files)
	return files, diagnoseMigrationFiles(files), nil
}

func diagnoseMigrationFiles(files []string) []MigrationDiagnostic {
	duplicates := duplicateMigrationNumericPrefixes(files)
	if len(duplicates) == 0 {
		return nil
	}

	prefixes := make([]string, 0, len(duplicates))
	for prefix := range duplicates {
		prefixes = append(prefixes, prefix)
	}
	slices.Sort(prefixes)

	diagnostics := make([]MigrationDiagnostic, 0, len(prefixes))
	for _, prefix := range prefixes {
		names := make([]string, len(duplicates[prefix]))
		copy(names, duplicates[prefix])
		slices.Sort(names)
		diagnostics = append(diagnostics, MigrationDiagnostic{
			Code:    "duplicate_numeric_prefix",
			Message: fmt.Sprintf("migration numeric prefix %s is used by multiple files", prefix),
			Files:   names,
		})
	}
	return diagnostics
}

func duplicateMigrationNumericPrefixes(files []string) map[string][]string {
	seen := map[string][]string{}
	for _, file := range files {
		prefix, ok := migrationNumericPrefix(file)
		if !ok {
			continue
		}
		seen[prefix] = append(seen[prefix], filepath.Base(file))
	}

	duplicates := map[string][]string{}
	for prefix, names := range seen {
		if len(names) > 1 {
			duplicates[prefix] = names
		}
	}
	return duplicates
}

func migrationNumericPrefix(file string) (string, bool) {
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	prefix, _, ok := strings.Cut(name, "_")
	if !ok || prefix == "" {
		return "", false
	}
	for _, r := range prefix {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return prefix, true
}

func (r *MigrationRunner) ApplyFile(ctx context.Context, file string) (MigrationResult, error) {
	sql, err := os.ReadFile(file)
	if err != nil {
		return MigrationResult{}, err
	}
	version, module := migrationVersion(file)
	result := MigrationResult{
		Version: version,
		Module:  module,
		Name:    filepath.Base(file),
	}
	checksum := checksum(sql)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("begin migration %s: %w", file, err)
	}
	defer tx.Rollback(ctx)

	var existingChecksum string
	var existingStatus string
	err = tx.QueryRow(ctx, `
		SELECT checksum, status
		FROM schema_migrations
		WHERE version = $1
	`, version).Scan(&existingChecksum, &existingStatus)
	if err == nil {
		if existingStatus == "applied" && existingChecksum != checksum {
			return result, fmt.Errorf("migration %s checksum mismatch", version)
		}
		if existingStatus == "applied" {
			result.Status = "skipped"
			return result, nil
		}
		return result, fmt.Errorf("migration %s has status %s", version, existingStatus)
	}
	if err != nil && err != pgx.ErrNoRows {
		return result, fmt.Errorf("read migration %s state: %w", version, err)
	}

	started := time.Now()
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		_ = tx.Rollback(ctx)
		_ = r.recordFailedMigration(ctx, version, module, filepath.Base(file), checksum, time.Since(started), err)
		result.Status = "failed"
		result.Error = err.Error()
		return result, fmt.Errorf("apply migration %s: %w", file, err)
	}
	elapsed := int(time.Since(started).Milliseconds())

	if _, err := tx.Exec(ctx, `
		INSERT INTO schema_migrations (version, module, name, checksum, status, error_message, execution_ms)
		VALUES ($1, $2, $3, $4, 'applied', '', $5)
		ON CONFLICT (version) DO UPDATE SET
			module = EXCLUDED.module,
			name = EXCLUDED.name,
			checksum = EXCLUDED.checksum,
			status = EXCLUDED.status,
			error_message = '',
			applied_at = now(),
			execution_ms = EXCLUDED.execution_ms
	`, version, module, filepath.Base(file), checksum, elapsed); err != nil {
		return result, fmt.Errorf("record migration %s: %w", file, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("commit migration %s: %w", file, err)
	}
	result.Status = "applied"
	result.ExecutionMS = elapsed
	return result, nil
}

func (r *MigrationRunner) ensureSchemaMigrations(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			module TEXT NOT NULL DEFAULT 'core',
			name TEXT NOT NULL,
			checksum TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'applied',
			error_message TEXT NOT NULL DEFAULT '',
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			execution_ms INTEGER NOT NULL DEFAULT 0
		);

		ALTER TABLE schema_migrations
			ADD COLUMN IF NOT EXISTS module TEXT NOT NULL DEFAULT 'core',
			ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS checksum TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'applied',
			ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			ADD COLUMN IF NOT EXISTS execution_ms INTEGER NOT NULL DEFAULT 0;
	`)
	return err
}

func (r *MigrationRunner) withMigrationLock(ctx context.Context, apply func(context.Context) error) error {
	pool, ok := r.db.(migrationLockDB)
	if !ok {
		return apply(ctx)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration lock connection: %w", err)
	}
	defer conn.Release()

	return withMigrationLockConn(ctx, conn, apply)
}

func withMigrationLockConn(ctx context.Context, conn migrationLockConn, apply func(context.Context) error) error {
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, migrationLockKey)
	}()

	return apply(ctx)
}

func (r *MigrationRunner) ensureNoFailedMigrations(ctx context.Context) error {
	rows, err := r.db.Query(ctx, `
		SELECT version, error_message
		FROM schema_migrations
		WHERE status = 'failed'
		ORDER BY version ASC
	`)
	if err != nil {
		return fmt.Errorf("read failed migrations: %w", err)
	}
	defer rows.Close()

	failed := []string{}
	for rows.Next() {
		var version string
		var message string
		if err := rows.Scan(&version, &message); err != nil {
			return err
		}
		if message != "" {
			failed = append(failed, version+": "+message)
		} else {
			failed = append(failed, version)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed migrations require manual repair: %s", strings.Join(failed, "; "))
	}
	return nil
}

func (r *MigrationRunner) recordFailedMigration(
	ctx context.Context,
	version string,
	module string,
	name string,
	checksum string,
	elapsed time.Duration,
	cause error,
) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO schema_migrations (version, module, name, checksum, status, error_message, execution_ms)
		VALUES ($1, $2, $3, $4, 'failed', $5, $6)
		ON CONFLICT (version) DO UPDATE SET
			module = EXCLUDED.module,
			name = EXCLUDED.name,
			checksum = EXCLUDED.checksum,
			status = EXCLUDED.status,
			error_message = EXCLUDED.error_message,
			execution_ms = EXCLUDED.execution_ms
	`, version, module, name, checksum, cause.Error(), int(elapsed.Milliseconds()))
	return err
}

func migrationVersion(file string) (string, string) {
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	parts := strings.Split(name, "_")
	if len(parts) < 2 {
		return name, "core"
	}

	switch parts[1] {
	case "auth":
		return name, "auth"
	case "job":
		return name, "jobs"
	case "proxy":
		return name, "proxy"
	case "server":
		return name, "servers"
	case "telegram":
		return name, "notifications"
	case "wiki":
		return name, "wiki"
	default:
		return name, parts[1]
	}
}

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
