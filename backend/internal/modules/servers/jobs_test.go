package servers

import (
	"context"
	"testing"

	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/queue"
)

type jobLogAppenderStub struct {
	jobID    string
	stream   string
	message  string
	metadata map[string]any
}

func (s *jobLogAppenderStub) AppendJobLog(ctx context.Context, id string, stream string, message string, metadata map[string]any) error {
	s.jobID = id
	s.stream = stream
	s.message = message
	s.metadata = metadata
	return nil
}

func TestServerCollectMaxAttemptsUsesQueueDefault(t *testing.T) {
	if ServerCollectMaxAttempts != queue.DefaultMaxAttempts {
		t.Fatalf("ServerCollectMaxAttempts = %d, want queue default %d", ServerCollectMaxAttempts, queue.DefaultMaxAttempts)
	}
}

func TestRegisterJobsConfiguresServerCollectRetries(t *testing.T) {
	registry := platformmodule.NewJobRegistry()
	if err := (Module{}).RegisterJobs(platformmodule.Context{}, registry); err != nil {
		t.Fatalf("RegisterJobs returned error: %v", err)
	}

	def, ok := registry.Definition("server.collect")
	if !ok {
		t.Fatal("server.collect job was not registered")
	}
	if def.MaxAttempts != ServerCollectMaxAttempts {
		t.Fatalf("MaxAttempts = %d, want %d", def.MaxAttempts, ServerCollectMaxAttempts)
	}
	if def.Handler == nil {
		t.Fatal("server.collect handler was nil")
	}
}

func TestCollectJobObserverWritesJobLogMetadata(t *testing.T) {
	logs := &jobLogAppenderStub{}
	observer := collectJobObserver{jobID: "job_1", logs: logs}

	observer.ObserveCollection(context.Background(), CollectionEvent{
		ServerID: "srv_1",
		Stage:    "fallback",
		Mode:     "ssh_once",
		Message:  "using fallback",
		Metadata: map[string]any{"error": "tcp blocked"},
	})

	if logs.jobID != "job_1" {
		t.Fatalf("jobID = %q, want job_1", logs.jobID)
	}
	if logs.stream != "system" {
		t.Fatalf("stream = %q, want system", logs.stream)
	}
	if logs.message != "using fallback" {
		t.Fatalf("message = %q, want using fallback", logs.message)
	}
	if logs.metadata["server_id"] != "srv_1" || logs.metadata["stage"] != "fallback" || logs.metadata["mode"] != "ssh_once" {
		t.Fatalf("metadata did not include collection fields: %#v", logs.metadata)
	}
	if logs.metadata["error"] != "tcp blocked" {
		t.Fatalf("metadata error = %#v, want tcp blocked", logs.metadata["error"])
	}
}
