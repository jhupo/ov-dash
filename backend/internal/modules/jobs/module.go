package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/platform/audit"
	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/queue"

	"github.com/go-chi/chi/v5"
)

type Module struct{}

type Handler struct {
	queue     jobQueue
	queueName string
	registry  *platformmodule.JobRegistry
	audit     audit.RequestRecorder
}

type jobQueue interface {
	Enqueue(ctx context.Context, queueName string, job queue.Job) error
	ListJobs(ctx context.Context, limit int) ([]queue.JobRecord, error)
	GetJob(ctx context.Context, id string) (queue.JobRecord, error)
	ListJobEvents(ctx context.Context, id string) ([]queue.JobEvent, error)
	ListJobLogs(ctx context.Context, id string) ([]queue.JobLog, error)
	RequestJobCancel(ctx context.Context, id string) error
	RequeueJob(ctx context.Context, queueName string, id string, idempotencyKey string) (queue.Job, error)
}

type CreateJobRequest struct {
	Type           string         `json:"type"`
	Payload        map[string]any `json:"payload"`
	IdempotencyKey string         `json:"idempotency_key"`
	ScheduledAt    *time.Time     `json:"scheduled_at"`
}

type RequeueJobRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

func NewModule() Module {
	return Module{}
}

func (Module) Manifest() platformmodule.Manifest {
	return platformmodule.Manifest{
		ID:          "jobs",
		Title:       "Jobs",
		Description: "Platform job registry, queue operations, job logs, cancellation, and requeue controls.",
		Kind:        "platform",
		Tags:        []string{"platform", "jobs", "worker"},
	}
}

func (Module) Register(reg *platformmodule.Registrar) error {
	if err := reg.HTTP(func(ctx platformmodule.Context) {
		handler := &Handler{
			queue:     ctx.Queue,
			queueName: ctx.Config.Worker.QueueName,
			registry:  ctx.Jobs,
			audit:     ctx.Audit,
		}
		ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsRead)).Get("/jobs", handler.List)
		ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsRead)).Get("/jobs/{id}", handler.Get)
		ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsRead)).Get("/jobs/{id}/events", handler.Events)
		ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsRead)).Get("/jobs/{id}/logs", handler.Logs)
		ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsCreate)).Post("/jobs", handler.Create)
		ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsManage)).Post("/jobs/{id}/cancel", handler.Cancel)
		ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsManage)).Post("/jobs/{id}/requeue", handler.Requeue)
	}); err != nil {
		return err
	}
	if err := reg.Job(platformmodule.JobDefinition{
		Type:        "noop",
		Description: "No-op job for platform smoke tests.",
		MaxAttempts: 1,
		Handler:     NoopHandler{},
	}); err != nil {
		return err
	}
	return reg.Capabilities(
		capability.JobsRead,
		capability.JobsCreate,
		capability.JobsManage,
	)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var input CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if input.Type == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "job_type_required"})
		return
	}
	var def platformmodule.JobDefinition
	if h.registry != nil {
		var ok bool
		def, ok = h.registry.Definition(input.Type)
		if !ok {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown_job_type"})
			return
		}
	}

	job, err := queue.NewJob(input.Type, input.Payload)
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	job.IdempotencyKey = input.IdempotencyKey
	job.ScheduledAt = input.ScheduledAt
	if def.MaxAttempts > 0 {
		job.MaxAttempts = def.MaxAttempts
	}
	job.Normalize()

	if err := h.queue.Enqueue(r.Context(), h.queueName, job); err != nil {
		if errors.Is(err, queue.ErrDuplicateIdempotencyKey) {
			httpx.WriteJSON(w, http.StatusConflict, map[string]string{"error": "duplicate_idempotency_key"})
			return
		}
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "enqueue_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, job)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			limit = value
		}
	}
	items, err := h.queue.ListJobs(r.Context(), limit)
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "jobs_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	item, err := h.queue.GetJob(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "job_not_found"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	items, err := h.queue.ListJobEvents(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "job_events_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	items, err := h.queue.ListJobLogs(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "job_logs_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	if err := h.queue.RequestJobCancel(r.Context(), jobID); err != nil {
		h.recordAudit(r, "jobs.cancel", jobID, "failure", err.Error(), nil)
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "job_cancel_failed"})
		return
	}
	h.recordAudit(r, "jobs.cancel", jobID, "success", "", nil)
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "cancel_requested"})
}

func (h *Handler) Requeue(w http.ResponseWriter, r *http.Request) {
	var input RequeueJobRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	oldJobID := chi.URLParam(r, "id")
	job, err := h.queue.RequeueJob(r.Context(), h.queueName, oldJobID, input.IdempotencyKey)
	if err != nil {
		h.recordAudit(r, "jobs.requeue", oldJobID, "failure", err.Error(), map[string]any{
			"idempotency_key": input.IdempotencyKey,
		})
		switch {
		case errors.Is(err, queue.ErrJobNotRequeueable):
			httpx.WriteJSON(w, http.StatusConflict, map[string]string{"error": "job_not_requeueable"})
		case errors.Is(err, queue.ErrDuplicateIdempotencyKey):
			httpx.WriteJSON(w, http.StatusConflict, map[string]string{"error": "duplicate_idempotency_key"})
		default:
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "job_requeue_failed"})
		}
		return
	}
	h.recordAudit(r, "jobs.requeue", oldJobID, "success", "", map[string]any{
		"new_job_id":      job.ID,
		"job_type":        job.Type,
		"idempotency_key": job.IdempotencyKey,
	})
	httpx.WriteJSON(w, http.StatusAccepted, job)
}

func (h *Handler) recordAudit(r *http.Request, action string, resourceID string, result string, message string, metadata map[string]any) {
	user, _ := auth.UserFromContext(r.Context())
	audit.RecordRequest(h.audit, r, audit.Entry{
		Actor: audit.Actor{
			ID:    user.ID,
			Email: user.Email,
			Role:  user.Role,
		},
		Action:     action,
		Resource:   "jobs",
		ResourceID: resourceID,
		Result:     result,
		Message:    message,
		Metadata:   metadata,
	})
}

type NoopHandler struct{}

func (NoopHandler) Handle(context.Context, queue.Job) error {
	return nil
}
