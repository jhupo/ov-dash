package queue

import (
	"strings"
	"testing"
	"time"
)

func TestJobNormalize(t *testing.T) {
	job := Job{
		IdempotencyKey: "  once ",
	}

	job.Normalize()

	if job.Payload == nil {
		t.Fatal("payload was not initialized")
	}
	if job.MaxAttempts != DefaultMaxAttempts {
		t.Fatalf("MaxAttempts = %d, want %d", job.MaxAttempts, DefaultMaxAttempts)
	}
	if job.CreatedAt.IsZero() {
		t.Fatal("CreatedAt was not initialized")
	}
	if job.IdempotencyKey != "once" {
		t.Fatalf("IdempotencyKey = %q, want %q", job.IdempotencyKey, "once")
	}
}

func TestJobNormalizePreservesExistingValues(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	payload := map[string]any{"ok": true}
	job := Job{
		Payload:     payload,
		MaxAttempts: 7,
		CreatedAt:   createdAt,
	}

	job.Normalize()

	if job.Payload["ok"] != true {
		t.Fatalf("Payload = %#v, want existing payload", job.Payload)
	}
	if job.MaxAttempts != 7 {
		t.Fatalf("MaxAttempts = %d, want 7", job.MaxAttempts)
	}
	if !job.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %s, want %s", job.CreatedAt, createdAt)
	}
}

func TestNewJobRejectsEmptyTypeAndNormalizesNilPayload(t *testing.T) {
	if _, err := NewJob("", nil); err == nil {
		t.Fatal("expected empty job type to be rejected")
	}

	job, err := NewJob("noop", nil)
	if err != nil {
		t.Fatalf("NewJob returned error: %v", err)
	}
	if job.Type != "noop" {
		t.Fatalf("Type = %q, want noop", job.Type)
	}
	if job.Payload == nil {
		t.Fatal("Payload was not initialized")
	}
	if len(job.ID) != 32 {
		t.Fatalf("ID length = %d, want 32", len(job.ID))
	}
	if strings.Trim(job.ID, "0123456789abcdef") != "" {
		t.Fatalf("ID = %q, want lowercase hex", job.ID)
	}
	if job.MaxAttempts != DefaultMaxAttempts {
		t.Fatalf("MaxAttempts = %d, want %d", job.MaxAttempts, DefaultMaxAttempts)
	}
	if job.CreatedAt.IsZero() {
		t.Fatal("CreatedAt was not initialized")
	}
}

func TestRedactJobLogMessage(t *testing.T) {
	got := redactJobLogMessage("password=secret token: abc123 visible=ok")

	for _, secret := range []string{"secret", "abc123"} {
		if strings.Contains(got, secret) {
			t.Fatalf("expected %q to be redacted from %q", secret, got)
		}
	}
	if !strings.Contains(got, "visible=ok") {
		t.Fatalf("expected non-sensitive text to be preserved, got %q", got)
	}
}
