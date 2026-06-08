package http

import (
	"context"
	"errors"
	"testing"
	"time"

	platformmodule "ov-dash/backend/internal/platform/module"
)

func TestRunPlatformHealthChecksAggregatesDownStatus(t *testing.T) {
	checkedAt := time.Date(2026, 6, 8, 1, 2, 3, 0, time.UTC)
	checks := []platformmodule.HealthCheck{
		{
			ID:   "database",
			Name: "Database",
			Check: func(context.Context) error {
				return nil
			},
		},
		{
			ID:   "worker",
			Name: "Worker heartbeat",
			Check: func(context.Context) error {
				return errors.New("no worker heartbeat")
			},
		},
	}

	got := runPlatformHealthChecks(context.Background(), checks, checkedAt)

	if got.Status != "down" {
		t.Fatalf("Status = %q, want down", got.Status)
	}
	if !got.CheckedAt.Equal(checkedAt) {
		t.Fatalf("CheckedAt = %s, want %s", got.CheckedAt, checkedAt)
	}
	if len(got.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(got.Items))
	}
	if got.Items[0].Status != "ok" {
		t.Fatalf("database status = %q, want ok", got.Items[0].Status)
	}
	if got.Items[1].Status != "down" {
		t.Fatalf("worker status = %q, want down", got.Items[1].Status)
	}
	if got.Items[1].Message != "no worker heartbeat" {
		t.Fatalf("worker message = %q", got.Items[1].Message)
	}
}

func TestRunPlatformHealthChecksReturnsOkWhenAllChecksPass(t *testing.T) {
	checks := []platformmodule.HealthCheck{
		{
			ID:   "database",
			Name: "Database",
			Check: func(context.Context) error {
				return nil
			},
		},
	}

	got := runPlatformHealthChecks(context.Background(), checks, time.Now().UTC())

	if got.Status != "ok" {
		t.Fatalf("Status = %q, want ok", got.Status)
	}
	if len(got.Items) != 1 || got.Items[0].Status != "ok" {
		t.Fatalf("unexpected items: %#v", got.Items)
	}
}
