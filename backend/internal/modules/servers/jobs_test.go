package servers

import (
	"context"
	"strings"
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

	updateDef, ok := registry.Definition("server.agent.update")
	if !ok {
		t.Fatal("server.agent.update job was not registered")
	}
	if updateDef.MaxAttempts != ServerAgentUpdateMaxAttempts {
		t.Fatalf("update MaxAttempts = %d, want %d", updateDef.MaxAttempts, ServerAgentUpdateMaxAttempts)
	}
	if updateDef.Handler == nil {
		t.Fatal("server.agent.update handler was nil")
	}
}

func TestNewAgentUpdateJobSetsStableIdempotencyKey(t *testing.T) {
	job, err := NewAgentUpdateJob("srv_1")
	if err != nil {
		t.Fatalf("NewAgentUpdateJob returned error: %v", err)
	}

	if job.Type != "server.agent.update" {
		t.Fatalf("Type = %q, want server.agent.update", job.Type)
	}
	if job.Payload["server_id"] != "srv_1" {
		t.Fatalf("payload = %#v", job.Payload)
	}
	if job.IdempotencyKey != "server.agent.update:srv_1" {
		t.Fatalf("IdempotencyKey = %q", job.IdempotencyKey)
	}
	if job.MaxAttempts != ServerAgentUpdateMaxAttempts {
		t.Fatalf("MaxAttempts = %d, want %d", job.MaxAttempts, ServerAgentUpdateMaxAttempts)
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

func TestAgentUpdateJobHandlerWritesInstallLogs(t *testing.T) {
	logs := &multiJobLogAppenderStub{}
	repository := newProbeRepositoryStub(Connection{ID: "srv_1"})
	probe := &serverProbeStub{
		installWithObserver: true,
		metric:              Metric{ServerID: "srv_1", LatencyMS: 15},
	}
	collector := &Collector{coordinator: NewProbeCoordinator(repository, probe)}
	handler := NewAgentUpdateJobHandler(collector, nil, logs)

	err := handler.Handle(context.Background(), queue.Job{
		ID:      "job_1",
		Type:    "server.agent.update",
		Payload: map[string]any{"server_id": "srv_1"},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if !containsLogStage(logs.entries, "agent.install.start") || !containsLogStage(logs.entries, "agent.install.collect_ok") {
		t.Fatalf("logs did not include install stages: %#v", logs.entries)
	}
	if len(repository.savedAgent) != 1 || repository.savedAgent[0].LatencyMS != 15 {
		t.Fatalf("saved agent metric = %#v", repository.savedAgent)
	}
}

func TestAgentUpdateJobHandlerRequiresServerID(t *testing.T) {
	handler := NewAgentUpdateJobHandler(&Collector{}, nil)

	err := handler.Handle(context.Background(), queue.Job{ID: "job_1", Type: "server.agent.update"})

	if err == nil || !strings.Contains(err.Error(), "server_id is required") {
		t.Fatalf("error = %v", err)
	}
}

type jobLogEntry struct {
	jobID    string
	stream   string
	message  string
	metadata map[string]any
}

type multiJobLogAppenderStub struct {
	entries []jobLogEntry
}

func (s *multiJobLogAppenderStub) AppendJobLog(ctx context.Context, id string, stream string, message string, metadata map[string]any) error {
	s.entries = append(s.entries, jobLogEntry{
		jobID:    id,
		stream:   stream,
		message:  message,
		metadata: metadata,
	})
	return nil
}

func containsLogStage(entries []jobLogEntry, stage string) bool {
	for _, entry := range entries {
		if entry.jobID == "job_1" && entry.metadata["stage"] == stage {
			return true
		}
	}
	return false
}
