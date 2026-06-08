package servers

import (
	"context"
	"net/http/httptest"
	"testing"

	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/platform/audit"
	"ov-dash/backend/internal/platform/redact"
)

func TestRecordAuditCapturesActorRequestAndMetadata(t *testing.T) {
	recorder := &fakeAuditRecorder{}
	handler := &Handler{audit: recorder}
	request := httptest.NewRequest("POST", "/server-connections/srv-1/ssh/command", nil)
	request = request.WithContext(auth.ContextWithUser(request.Context(), auth.User{
		ID:    "user-1",
		Email: "ops@example.com",
		Role:  "admin",
	}))
	request.Header.Set("User-Agent", "servers-test")
	request.Header.Set("X-Forwarded-For", "203.0.113.20, 192.0.2.10")

	handler.recordAudit(request, "servers.ssh.command", "srv-1", "success", "", map[string]any{
		"command":        redact.Text("deploy password=super-secret"),
		"command_length": len("deploy password=super-secret"),
	})

	entry := recorder.single(t)
	if entry.Action != "servers.ssh.command" || entry.Resource != "servers" || entry.ResourceID != "srv-1" || entry.Result != "success" {
		t.Fatalf("audit entry did not describe server command: %#v", entry)
	}
	if entry.Actor.ID != "user-1" || entry.Actor.Email != "ops@example.com" || entry.Actor.Role != "admin" {
		t.Fatalf("audit actor = %#v", entry.Actor)
	}
	if entry.IP != "203.0.113.20" || entry.UserAgent != "servers-test" {
		t.Fatalf("request audit fields = ip %q ua %q", entry.IP, entry.UserAgent)
	}
	if entry.Metadata["command"] == "deploy password=super-secret" {
		t.Fatalf("command metadata was not redacted: %#v", entry.Metadata)
	}
	if entry.Metadata["command_length"] != len("deploy password=super-secret") {
		t.Fatalf("command length metadata = %#v", entry.Metadata["command_length"])
	}
}

type fakeAuditRecorder struct {
	entries []audit.Entry
}

func (r *fakeAuditRecorder) Record(_ context.Context, entry audit.Entry) error {
	r.entries = append(r.entries, entry)
	return nil
}

func (r *fakeAuditRecorder) single(t *testing.T) audit.Entry {
	t.Helper()
	if len(r.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1: %#v", len(r.entries), r.entries)
	}
	return r.entries[0]
}
