package jobs

import (
	"encoding/json"
	"net/http"

	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/queue"
)

type Module struct{}

type Handler struct {
	events    *events.Bus
	queue     *queue.Client
	queueName string
}

type CreateJobRequest struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

func NewModule() Module {
	return Module{}
}

func (Module) Name() string {
	return "jobs"
}

func (Module) RegisterRoutes(ctx platformmodule.Context) {
	handler := &Handler{
		events:    ctx.Events,
		queue:     ctx.Queue,
		queueName: ctx.Config.Worker.QueueName,
	}
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.JobsCreate)).Post("/jobs", handler.Create)
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

	job, err := queue.NewJob(input.Type, input.Payload)
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.queue.Enqueue(r.Context(), h.queueName, job); err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "enqueue_failed"})
		return
	}
	h.events.Publish(r.Context(), events.New("job.enqueued", "http.jobs", map[string]any{
		"job_id":   job.ID,
		"job_type": job.Type,
	}))

	httpx.WriteJSON(w, http.StatusAccepted, job)
}
