package http

import (
	"encoding/json"
	"net/http"

	"ov-dash/backend/internal/events"
	"ov-dash/backend/internal/platform"
	"ov-dash/backend/internal/queue"
)

type JobsHandler struct {
	runtime *platform.Runtime
}

type CreateJobRequest struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

func NewJobsHandler(runtime *platform.Runtime) *JobsHandler {
	return &JobsHandler{runtime: runtime}
}

func (h *JobsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if input.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job_type_required"})
		return
	}

	job, err := queue.NewJob(input.Type, input.Payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.runtime.Queue.Enqueue(r.Context(), h.runtime.Config.Worker.QueueName, job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "enqueue_failed"})
		return
	}
	h.runtime.Events.Publish(r.Context(), events.New("job.enqueued", "http.jobs", map[string]any{
		"job_id":   job.ID,
		"job_type": job.Type,
	}))

	writeJSON(w, http.StatusAccepted, job)
}
