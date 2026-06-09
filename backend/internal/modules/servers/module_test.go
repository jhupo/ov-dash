package servers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/platform/audit"
	"ov-dash/backend/internal/platform/redact"
	"ov-dash/backend/internal/queue"

	"github.com/go-chi/chi/v5"
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

func TestUpdateAgentEnqueuesJobAndRecordsAudit(t *testing.T) {
	auditRecorder := &fakeAuditRecorder{}
	fakeQueue := &fakeServerJobQueue{}
	handler := &Handler{
		queue:     fakeQueue,
		queueName: "jobs:test",
		audit:     auditRecorder,
	}
	recorder := httptest.NewRecorder()
	request := serverRequestWithID(http.MethodPost, "/server-connections/srv-1/agent/update", "srv-1")
	request = request.WithContext(auth.ContextWithUser(request.Context(), auth.User{
		ID:    "user-1",
		Email: "ops@example.com",
		Role:  "admin",
	}))

	handler.UpdateAgent(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	if fakeQueue.queueName != "jobs:test" {
		t.Fatalf("queue name = %q", fakeQueue.queueName)
	}
	if fakeQueue.job.Type != "server.agent.update" || fakeQueue.job.Payload["server_id"] != "srv-1" {
		t.Fatalf("job = %#v", fakeQueue.job)
	}
	if fakeQueue.job.IdempotencyKey != ServerAgentUpdateIdempotencyKey("srv-1") {
		t.Fatalf("idempotency key = %q", fakeQueue.job.IdempotencyKey)
	}
	entry := auditRecorder.single(t)
	if entry.Action != "servers.agent.update" || entry.Result != "success" || entry.ResourceID != "srv-1" {
		t.Fatalf("audit entry = %#v", entry)
	}
	if entry.Metadata["job_id"] != fakeQueue.job.ID || entry.Metadata["idempotency_key"] != fakeQueue.job.IdempotencyKey {
		t.Fatalf("audit metadata = %#v", entry.Metadata)
	}
}

func TestUpdateAgentMapsDuplicateJob(t *testing.T) {
	auditRecorder := &fakeAuditRecorder{}
	handler := &Handler{
		queue:     &fakeServerJobQueue{err: queue.ErrDuplicateIdempotencyKey},
		queueName: "jobs:test",
		audit:     auditRecorder,
	}
	recorder := httptest.NewRecorder()
	request := serverRequestWithID(http.MethodPost, "/server-connections/srv-1/agent/update", "srv-1")

	handler.UpdateAgent(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	entry := auditRecorder.single(t)
	if entry.Action != "servers.agent.update" || entry.Result != "failure" {
		t.Fatalf("audit entry = %#v", entry)
	}
}

func serverRequestWithID(method string, target string, id string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewBuffer(nil))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", id)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeCtx))
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

type fakeServerJobQueue struct {
	queueName string
	job       queue.Job
	err       error
}

func (q *fakeServerJobQueue) Enqueue(_ context.Context, queueName string, job queue.Job) error {
	if q.err != nil {
		return q.err
	}
	if job.ID == "" {
		return errors.New("job id is required")
	}
	q.queueName = queueName
	q.job = job
	return nil
}
