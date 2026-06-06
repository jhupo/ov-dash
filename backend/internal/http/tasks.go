package http

import (
	"encoding/json"
	"net/http"

	"ov-dash/backend/internal/modules/tasks"
)

type TasksHandler struct {
	service *tasks.Service
}

func NewTasksHandler(service *tasks.Service) *TasksHandler {
	return &TasksHandler{service: service}
}

func (h *TasksHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tasks_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type deleteTasksRequest struct {
	IDs []string `json:"ids"`
}

func (h *TasksHandler) Delete(w http.ResponseWriter, r *http.Request) {
	var payload deleteTasksRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if err := h.service.Delete(r.Context(), payload.IDs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tasks_delete_failed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
