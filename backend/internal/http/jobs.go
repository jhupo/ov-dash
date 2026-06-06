package http

import (
	"encoding/json"
	"net/http"

	"ov-dash/backend/internal/config"
	"ov-dash/backend/internal/queue"
)

type JobsHandler struct {
	queue  *queue.Client
	worker config.WorkerConfig
}

type CreateJobRequest struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

func NewJobsHandler(queue *queue.Client, worker config.WorkerConfig) *JobsHandler {
	return &JobsHandler{queue: queue, worker: worker}
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

	if err := h.queue.Enqueue(r.Context(), h.worker.QueueName, job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "enqueue_failed"})
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}
