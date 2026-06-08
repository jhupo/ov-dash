package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/platform/redact"
	"ov-dash/backend/internal/queue"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Module struct{}

type Handler struct {
	events    *events.Bus
	queue     jobQueue
	queueName string
	registry  *platformmodule.JobRegistry
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
}

type RequeueJobRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

func NewModule() Module {
	return Module{}
}

func (Module) ID() string {
	return "jobs"
}

func (Module) RegisterHTTP(ctx platformmodule.Context) {
	handler := &Handler{
		events:    ctx.Events,
		queue:     ctx.Queue,
		queueName: ctx.Config.Worker.QueueName,
		registry:  ctx.Jobs,
	}
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsRead)).Get("/jobs", handler.List)
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsRead)).Get("/jobs/{id}", handler.Get)
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsRead)).Get("/jobs/{id}/events", handler.Events)
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsRead)).Get("/jobs/{id}/logs", handler.Logs)
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsCreate)).Post("/jobs", handler.Create)
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsManage)).Post("/jobs/{id}/cancel", handler.Cancel)
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsManage)).Post("/jobs/{id}/requeue", handler.Requeue)
}

func (Module) RegisterJobs(ctx platformmodule.Context, reg *platformmodule.JobRegistry) error {
	if err := reg.Register(platformmodule.JobDefinition{
		Type:        "python.script",
		Description: "Run a Python script from the configured scripts directory.",
		MaxAttempts: queue.DefaultMaxAttempts,
		Handler:     NewPythonScriptHandler(ctx.Config.Python, ctx.Logger, ctx.Queue),
	}); err != nil {
		return err
	}
	return reg.Register(platformmodule.JobDefinition{
		Type:        "noop",
		Description: "No-op job for platform smoke tests.",
		MaxAttempts: 1,
		Handler:     NoopHandler{},
	})
}

func (Module) Capabilities() []capability.Capability {
	return []capability.Capability{
		capability.JobsRead,
		capability.JobsCreate,
		capability.JobsManage,
	}
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
	h.events.Publish(r.Context(), events.New("job.enqueued", "http.jobs", map[string]any{
		"job_id":   job.ID,
		"job_type": job.Type,
	}))

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
	if err := h.queue.RequestJobCancel(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "job_cancel_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "cancel_requested"})
}

func (h *Handler) Requeue(w http.ResponseWriter, r *http.Request) {
	var input RequeueJobRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	job, err := h.queue.RequeueJob(r.Context(), h.queueName, chi.URLParam(r, "id"), input.IdempotencyKey)
	if err != nil {
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
	httpx.WriteJSON(w, http.StatusAccepted, job)
}

type PythonScriptHandler struct {
	cfg    config.PythonConfig
	logger *zap.Logger
	logs   jobLogAppender
}

type jobLogAppender interface {
	AppendJobLog(ctx context.Context, id string, stream string, message string, metadata map[string]any) error
}

type pythonScriptPayload struct {
	Script string         `json:"script"`
	Args   []string       `json:"args"`
	Input  map[string]any `json:"input"`
}

func NewPythonScriptHandler(cfg config.PythonConfig, logger *zap.Logger, logs ...jobLogAppender) *PythonScriptHandler {
	handler := &PythonScriptHandler{cfg: cfg, logger: logger}
	if len(logs) > 0 {
		handler.logs = logs[0]
	}
	return handler
}

func (h *PythonScriptHandler) Handle(ctx context.Context, job queue.Job) error {
	var payload pythonScriptPayload
	raw, err := json.Marshal(job.Payload)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	if payload.Script == "" {
		return errors.New("script is required")
	}
	if !filepath.IsLocal(payload.Script) {
		return fmt.Errorf("script path must be local: %s", payload.Script)
	}
	scriptPath := filepath.Clean(filepath.Join(h.cfg.ScriptsDir, payload.Script))

	args := append([]string{scriptPath}, payload.Args...)
	cmd := exec.CommandContext(ctx, h.cfg.Bin, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	h.recordScriptOutput(ctx, job, payload.Script, stdout.Bytes(), stderr.Bytes())
	if err != nil {
		return fmt.Errorf("run python script: %w", err)
	}
	return nil
}

func (h *PythonScriptHandler) recordScriptOutput(ctx context.Context, job queue.Job, script string, stdout []byte, stderr []byte) {
	if len(stdout) > 0 && h.logger != nil {
		h.logger.Info("python script stdout", zap.String("job_id", job.ID), zap.String("output", redact.Text(strings.ToValidUTF8(string(stdout), "\uFFFD"))))
	}
	if len(stderr) > 0 && h.logger != nil {
		h.logger.Info("python script stderr", zap.String("job_id", job.ID), zap.String("output", redact.Text(strings.ToValidUTF8(string(stderr), "\uFFFD"))))
	}
	if h.logs == nil {
		return
	}
	metadata := map[string]any{
		"handler": "python.script",
		"script":  script,
	}
	h.appendScriptLog(ctx, job.ID, "stdout", stdout, metadata)
	h.appendScriptLog(ctx, job.ID, "stderr", stderr, metadata)
}

func (h *PythonScriptHandler) appendScriptLog(ctx context.Context, jobID string, stream string, output []byte, metadata map[string]any) {
	if len(output) == 0 {
		return
	}
	message := strings.ToValidUTF8(string(output), "\uFFFD")
	if err := h.logs.AppendJobLog(ctx, jobID, stream, strings.TrimRight(message, "\r\n"), metadata); err != nil && h.logger != nil {
		h.logger.Error("append python script output job log", zap.String("job_id", jobID), zap.String("stream", stream), zap.Error(err))
	}
}

type NoopHandler struct{}

func (NoopHandler) Handle(context.Context, queue.Job) error {
	return nil
}
