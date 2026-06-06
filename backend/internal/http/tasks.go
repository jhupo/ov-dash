package http

import (
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
