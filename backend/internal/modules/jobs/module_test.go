package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/platform/audit"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/queue"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func TestCreateMapsValidationAndQueueErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		setup      func(*fakeJobQueue, *platformmodule.JobRegistry)
		wantStatus int
		wantError  string
	}{
		{
			name:       "invalid json",
			body:       "{",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_json",
		},
		{
			name:       "missing type",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "job_type_required",
		},
		{
			name:       "unknown type",
			body:       `{"type":"missing"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "unknown_job_type",
		},
		{
			name: "duplicate idempotency key",
			body: `{"type":"noop","idempotency_key":"once"}`,
			setup: func(q *fakeJobQueue, reg *platformmodule.JobRegistry) {
				registerNoop(t, reg, 1)
				q.enqueueErr = queue.ErrDuplicateIdempotencyKey
			},
			wantStatus: http.StatusConflict,
			wantError:  "duplicate_idempotency_key",
		},
		{
			name: "enqueue failure",
			body: `{"type":"noop"}`,
			setup: func(q *fakeJobQueue, reg *platformmodule.JobRegistry) {
				registerNoop(t, reg, 1)
				q.enqueueErr = errors.New("redis unavailable")
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "enqueue_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeQueue := &fakeJobQueue{}
			registry := platformmodule.NewJobRegistry()
			if tt.setup != nil {
				tt.setup(fakeQueue, registry)
			}
			handler := newTestHandler(fakeQueue, registry)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(tt.body))

			handler.Create(recorder, request)

			assertJSONError(t, recorder, tt.wantStatus, tt.wantError)
		})
	}
}

func TestCreateEnqueuesNormalizedJobAndPublishesEvent(t *testing.T) {
	fakeQueue := &fakeJobQueue{}
	registry := platformmodule.NewJobRegistry()
	registerNoop(t, registry, 1)
	handler := newTestHandler(fakeQueue, registry)
	published := []events.Event{}
	handler.events.SubscribeAll(func(_ context.Context, event events.Event) error {
		published = append(published, event)
		return nil
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{
		"type": "noop",
		"payload": {"ok": true},
		"idempotency_key": " once "
	}`))

	handler.Create(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	if fakeQueue.enqueued == nil {
		t.Fatal("expected job to be enqueued")
	}
	if fakeQueue.enqueued.queueName != "jobs:test" {
		t.Fatalf("queue name = %q, want jobs:test", fakeQueue.enqueued.queueName)
	}
	if fakeQueue.enqueued.job.IdempotencyKey != "once" {
		t.Fatalf("idempotency key = %q, want once", fakeQueue.enqueued.job.IdempotencyKey)
	}
	if fakeQueue.enqueued.job.MaxAttempts != 1 {
		t.Fatalf("max attempts = %d, want registry override 1", fakeQueue.enqueued.job.MaxAttempts)
	}
	if len(published) != 1 || published[0].Type != "job.enqueued" {
		t.Fatalf("published events = %#v, want one job.enqueued", published)
	}
}

func TestRequeueMapsQueueErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
		wantError  string
	}{
		{
			name:       "invalid json",
			body:       "{",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_json",
		},
		{
			name:       "not requeueable",
			body:       `{}`,
			err:        queue.ErrJobNotRequeueable,
			wantStatus: http.StatusConflict,
			wantError:  "job_not_requeueable",
		},
		{
			name:       "duplicate idempotency key",
			body:       `{"idempotency_key":"once"}`,
			err:        queue.ErrDuplicateIdempotencyKey,
			wantStatus: http.StatusConflict,
			wantError:  "duplicate_idempotency_key",
		},
		{
			name:       "generic failure",
			body:       `{}`,
			err:        errors.New("db down"),
			wantStatus: http.StatusInternalServerError,
			wantError:  "job_requeue_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeQueue := &fakeJobQueue{requeueErr: tt.err}
			handler := newTestHandler(fakeQueue, platformmodule.NewJobRegistry())
			recorder := httptest.NewRecorder()
			request := requestWithJobID(http.MethodPost, "/jobs/job-1/requeue", tt.body, "job-1")

			handler.Requeue(recorder, request)

			assertJSONError(t, recorder, tt.wantStatus, tt.wantError)
		})
	}
}

func TestCancelMapsQueueFailure(t *testing.T) {
	handler := newTestHandler(&fakeJobQueue{cancelErr: errors.New("db down")}, platformmodule.NewJobRegistry())
	recorder := httptest.NewRecorder()
	request := requestWithJobID(http.MethodPost, "/jobs/job-1/cancel", "", "job-1")

	handler.Cancel(recorder, request)

	assertJSONError(t, recorder, http.StatusInternalServerError, "job_cancel_failed")
}

func TestCancelRecordsAudit(t *testing.T) {
	auditRecorder := &fakeAuditRecorder{}
	handler := newTestHandler(&fakeJobQueue{}, platformmodule.NewJobRegistry())
	handler.audit = auditRecorder
	recorder := httptest.NewRecorder()
	request := requestWithUser(requestWithJobID(http.MethodPost, "/jobs/job-1/cancel", "", "job-1"))
	request.Header.Set("User-Agent", "jobs-test")
	request.RemoteAddr = "192.0.2.10:12345"

	handler.Cancel(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	entry := auditRecorder.single(t)
	if entry.Action != "jobs.cancel" || entry.Resource != "jobs" || entry.ResourceID != "job-1" || entry.Result != "success" {
		t.Fatalf("audit entry did not describe cancel: %#v", entry)
	}
	if entry.Actor.ID != "user-1" || entry.Actor.Email != "ops@example.com" || entry.Actor.Role != "admin" {
		t.Fatalf("audit actor = %#v", entry.Actor)
	}
	if entry.IP != "192.0.2.10" || entry.UserAgent != "jobs-test" {
		t.Fatalf("request audit fields = ip %q ua %q", entry.IP, entry.UserAgent)
	}
}

func TestRequeueRecordsAudit(t *testing.T) {
	auditRecorder := &fakeAuditRecorder{}
	fakeQueue := &fakeJobQueue{
		requeueJob: queue.Job{
			ID:             "new-job",
			Type:           "noop",
			IdempotencyKey: "once",
			CreatedAt:      time.Now().UTC(),
		},
	}
	handler := newTestHandler(fakeQueue, platformmodule.NewJobRegistry())
	handler.audit = auditRecorder
	recorder := httptest.NewRecorder()
	request := requestWithUser(requestWithJobID(http.MethodPost, "/jobs/job-1/requeue", `{"idempotency_key":"once"}`, "job-1"))
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 192.0.2.10")

	handler.Requeue(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	entry := auditRecorder.single(t)
	if entry.Action != "jobs.requeue" || entry.ResourceID != "job-1" || entry.Result != "success" {
		t.Fatalf("audit entry did not describe requeue: %#v", entry)
	}
	if entry.Metadata["new_job_id"] != "new-job" || entry.Metadata["job_type"] != "noop" || entry.Metadata["idempotency_key"] != "once" {
		t.Fatalf("audit metadata = %#v", entry.Metadata)
	}
	if entry.IP != "203.0.113.9" {
		t.Fatalf("audit ip = %q, want forwarded address", entry.IP)
	}
}

func newTestHandler(q *fakeJobQueue, registry *platformmodule.JobRegistry) *Handler {
	return &Handler{
		events:    events.NewBus(zap.NewNop()),
		queue:     q,
		queueName: "jobs:test",
		registry:  registry,
	}
}

func registerNoop(t *testing.T, registry *platformmodule.JobRegistry, maxAttempts int) {
	t.Helper()
	if err := registry.Register(platformmodule.JobDefinition{
		Type:        "noop",
		MaxAttempts: maxAttempts,
		Handler:     NoopHandler{},
	}); err != nil {
		t.Fatalf("register noop job: %v", err)
	}
}

func requestWithJobID(method string, target string, body string, id string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", id)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeCtx))
}

func requestWithUser(request *http.Request) *http.Request {
	return request.WithContext(auth.ContextWithUser(request.Context(), auth.User{
		ID:    "user-1",
		Email: "ops@example.com",
		Role:  "admin",
	}))
}

func assertJSONError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, status, recorder.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body["error"] != code {
		t.Fatalf("error = %q, want %q", body["error"], code)
	}
}

type fakeJobQueue struct {
	enqueueErr error
	requeueErr error
	cancelErr  error
	enqueued   *fakeEnqueuedJob
	requeueJob queue.Job
}

type fakeEnqueuedJob struct {
	queueName string
	job       queue.Job
}

func (q *fakeJobQueue) Enqueue(_ context.Context, queueName string, job queue.Job) error {
	if q.enqueueErr != nil {
		return q.enqueueErr
	}
	q.enqueued = &fakeEnqueuedJob{queueName: queueName, job: job}
	return nil
}

func (q *fakeJobQueue) ListJobs(context.Context, int) ([]queue.JobRecord, error) {
	return nil, errors.New("not implemented")
}

func (q *fakeJobQueue) GetJob(context.Context, string) (queue.JobRecord, error) {
	return queue.JobRecord{}, errors.New("not implemented")
}

func (q *fakeJobQueue) ListJobEvents(context.Context, string) ([]queue.JobEvent, error) {
	return nil, errors.New("not implemented")
}

func (q *fakeJobQueue) ListJobLogs(context.Context, string) ([]queue.JobLog, error) {
	return nil, errors.New("not implemented")
}

func (q *fakeJobQueue) RequestJobCancel(context.Context, string) error {
	return q.cancelErr
}

func (q *fakeJobQueue) RequeueJob(context.Context, string, string, string) (queue.Job, error) {
	if q.requeueErr != nil {
		return queue.Job{}, q.requeueErr
	}
	if q.requeueJob.ID != "" {
		return q.requeueJob, nil
	}
	return queue.Job{ID: "new-job", Type: "noop", CreatedAt: time.Now().UTC()}, nil
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
