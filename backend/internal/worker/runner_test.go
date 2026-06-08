package worker

import (
	"context"
	"testing"
	"time"

	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/queue"
)

func TestRetryBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 5 * time.Second},
		{attempt: 1, want: 5 * time.Second},
		{attempt: 2, want: 10 * time.Second},
		{attempt: 3, want: 20 * time.Second},
		{attempt: 5, want: time.Minute},
		{attempt: 9, want: time.Minute},
	}

	for _, tt := range tests {
		if got := retryBackoff(tt.attempt); got != tt.want {
			t.Fatalf("retryBackoff(%d) = %s, want %s", tt.attempt, got, tt.want)
		}
	}
}

func TestNewRegisteredJobUsesRegisteredMaxAttempts(t *testing.T) {
	registry := platformmodule.NewJobRegistry()
	if err := registry.Register(platformmodule.JobDefinition{
		Type:        "server.collect",
		MaxAttempts: 5,
		Handler:     fakeJobHandler{},
	}); err != nil {
		t.Fatalf("register job: %v", err)
	}

	runner := &Runner{jobs: registry}
	job, err := runner.newRegisteredJob("server.collect", map[string]any{"server_id": "srv_1"})
	if err != nil {
		t.Fatalf("newRegisteredJob returned error: %v", err)
	}

	if job.MaxAttempts != 5 {
		t.Fatalf("MaxAttempts = %d, want 5", job.MaxAttempts)
	}
	if job.Type != "server.collect" {
		t.Fatalf("Type = %q, want server.collect", job.Type)
	}
	if job.Payload["server_id"] != "srv_1" {
		t.Fatalf("server_id payload = %v, want srv_1", job.Payload["server_id"])
	}
}

func TestNewRegisteredJobFallsBackToQueueDefault(t *testing.T) {
	runner := &Runner{jobs: platformmodule.NewJobRegistry()}
	job, err := runner.newRegisteredJob("server.collect", nil)
	if err != nil {
		t.Fatalf("newRegisteredJob returned error: %v", err)
	}

	if job.MaxAttempts != queue.DefaultMaxAttempts {
		t.Fatalf("MaxAttempts = %d, want queue default %d", job.MaxAttempts, queue.DefaultMaxAttempts)
	}
	if job.Payload == nil {
		t.Fatal("Payload is nil, want normalized empty map")
	}
}

type fakeJobHandler struct{}

func (fakeJobHandler) Handle(context.Context, queue.Job) error {
	return nil
}
