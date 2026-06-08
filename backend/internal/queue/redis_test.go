package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewRequeuedJobUsesDistinctIDAndMetadata(t *testing.T) {
	record := JobRecord{
		ID:             "old-job",
		QueueName:      "jobs",
		Type:           "noop",
		Payload:        map[string]any{"ok": true},
		Status:         JobStatusDead,
		MaxAttempts:    7,
		IdempotencyKey: "old-key",
	}

	job, err := newRequeuedJob(record, "new-key")
	if err != nil {
		t.Fatalf("newRequeuedJob returned error: %v", err)
	}

	if job.ID == "" {
		t.Fatal("expected new job id to be set")
	}
	if job.ID == record.ID {
		t.Fatal("expected requeued job to use a new id")
	}
	if job.Type != record.Type {
		t.Fatalf("expected job type %q, got %q", record.Type, job.Type)
	}
	if job.MaxAttempts != record.MaxAttempts {
		t.Fatalf("expected max attempts %d, got %d", record.MaxAttempts, job.MaxAttempts)
	}
	if job.IdempotencyKey != "new-key" {
		t.Fatalf("expected new idempotency key, got %q", job.IdempotencyKey)
	}

	metadata := requeueMetadata("jobs", record, job)
	if metadata["old_job_id"] != record.ID {
		t.Fatalf("expected old_job_id %q, got %v", record.ID, metadata["old_job_id"])
	}
	if metadata["new_job_id"] != job.ID {
		t.Fatalf("expected new_job_id %q, got %v", job.ID, metadata["new_job_id"])
	}
	if metadata["old_status"] != JobStatusDead {
		t.Fatalf("expected old_status %q, got %v", JobStatusDead, metadata["old_status"])
	}
}

func TestRecordEnqueueAfterAuditFailureMarksFailedAndLogs(t *testing.T) {
	audit := &recordingAuditStore{}
	client := &Client{audit: audit}
	job := Job{ID: "job-1", Type: "noop", Payload: map[string]any{}, MaxAttempts: 1}
	redisErr := errors.New("redis unavailable")

	err := client.recordEnqueueAfterAuditFailure(context.Background(), "jobs", job, "redis_push", redisErr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed after audit record") {
		t.Fatalf("expected error to mention audit had already been recorded, got %q", err.Error())
	}
	if audit.failedJobID != job.ID {
		t.Fatalf("expected failed audit for %q, got %q", job.ID, audit.failedJobID)
	}
	if !strings.Contains(audit.failedMessage, "redis_push") {
		t.Fatalf("expected failed message to include phase, got %q", audit.failedMessage)
	}
	if len(audit.logs) != 1 {
		t.Fatalf("expected one job log, got %d", len(audit.logs))
	}
	if audit.logs[0].jobID != job.ID {
		t.Fatalf("expected log for job %q, got %q", job.ID, audit.logs[0].jobID)
	}
	if audit.logs[0].stream != "system" {
		t.Fatalf("expected system log, got %q", audit.logs[0].stream)
	}
	if audit.logs[0].metadata["phase"] != "redis_push" {
		t.Fatalf("expected phase metadata, got %#v", audit.logs[0].metadata)
	}
}

type recordingAuditStore struct {
	failedJobID   string
	failedMessage string
	logs          []recordedJobLog
}

type recordedJobLog struct {
	jobID    string
	stream   string
	message  string
	metadata map[string]any
}

func (s *recordingAuditStore) RecordJobEnqueued(context.Context, string, Job) error {
	return nil
}

func (s *recordingAuditStore) RecordJobRunning(context.Context, string, Job) error {
	return nil
}

func (s *recordingAuditStore) RecordJobCompleted(context.Context, string, Job) error {
	return nil
}

func (s *recordingAuditStore) RecordJobFailed(_ context.Context, _ string, job Job, message string, _ *time.Time) error {
	s.failedJobID = job.ID
	s.failedMessage = message
	return nil
}

func (s *recordingAuditStore) RecordJobDead(context.Context, string, Job, string) error {
	return nil
}

func (s *recordingAuditStore) RecordJobCanceled(context.Context, string, Job, string) error {
	return nil
}

func (s *recordingAuditStore) RecordJobRequeued(context.Context, string, JobRecord, Job) error {
	return nil
}

func (s *recordingAuditStore) ListJobs(context.Context, int) ([]JobRecord, error) {
	return nil, nil
}

func (s *recordingAuditStore) GetJob(context.Context, string) (JobRecord, error) {
	return JobRecord{}, nil
}

func (s *recordingAuditStore) ListJobEvents(context.Context, string) ([]JobEvent, error) {
	return nil, nil
}

func (s *recordingAuditStore) AppendJobLog(_ context.Context, id string, stream string, message string, metadata map[string]any) error {
	s.logs = append(s.logs, recordedJobLog{
		jobID:    id,
		stream:   stream,
		message:  message,
		metadata: metadata,
	})
	return nil
}

func (s *recordingAuditStore) ListJobLogs(context.Context, string) ([]JobLog, error) {
	return nil, nil
}

func (s *recordingAuditStore) RequestJobCancel(context.Context, string) error {
	return nil
}

func (s *recordingAuditStore) IsJobCancelRequested(context.Context, string) (bool, error) {
	return false, nil
}

func (s *recordingAuditStore) RecordWorkerHeartbeat(context.Context, string, string, string, int) error {
	return nil
}

func (s *recordingAuditStore) ListWorkerHeartbeats(context.Context, time.Duration) ([]WorkerHeartbeat, error) {
	return nil, nil
}
