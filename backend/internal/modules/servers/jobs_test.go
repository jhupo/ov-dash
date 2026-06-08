package servers

import (
	"testing"

	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/queue"
)

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
