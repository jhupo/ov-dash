package queue

import (
	"testing"
	"time"
)

func TestJobNormalizeDefaults(t *testing.T) {
	job := Job{}

	job.Normalize()

	if job.Payload == nil {
		t.Fatal("expected payload to be initialized")
	}
	if job.MaxAttempts != DefaultMaxAttempts {
		t.Fatalf("expected max attempts %d, got %d", DefaultMaxAttempts, job.MaxAttempts)
	}
	if job.CreatedAt.IsZero() {
		t.Fatal("expected created at to be set")
	}
}

func TestJobNormalizePreservesValues(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	job := Job{
		Payload:     map[string]any{"ok": true},
		MaxAttempts: 7,
		CreatedAt:   createdAt,
	}

	job.Normalize()

	if job.MaxAttempts != 7 {
		t.Fatalf("expected max attempts to be preserved, got %d", job.MaxAttempts)
	}
	if !job.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected created at to be preserved, got %s", job.CreatedAt)
	}
}
