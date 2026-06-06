package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"ov-dash/backend/internal/db"
)

type MigrationRunner struct {
	db *db.Pool
}

func NewMigrationRunner(db *db.Pool) *MigrationRunner {
	return &MigrationRunner{db: db}
}

func (r *MigrationRunner) ApplyDir(ctx context.Context, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	slices.Sort(files)

	for _, file := range files {
		if err := r.ApplyFile(ctx, file); err != nil {
			return err
		}
	}
	return nil
}

func (r *MigrationRunner) ApplyFile(ctx context.Context, file string) error {
	sql, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("apply migration %s: %w", file, err)
	}
	return nil
}
