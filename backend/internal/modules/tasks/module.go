package tasks

import (
	"encoding/json"
	"net/http"

	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
)

type Module struct{}

type Handler struct {
	service *Service
}

type deleteTasksRequest struct {
	IDs []string `json:"ids"`
}

func NewModule() Module {
	return Module{}
}

func (Module) ID() string {
	return "tasks"
}

func (Module) RegisterHTTP(ctx platformmodule.Context) {
	handler := &Handler{service: NewService(NewRepository(ctx.DB))}
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.TasksRead)).Get("/tasks", handler.List)
	ctx.ProtectedRouter.With(ctx.RequireCapability(capability.TasksWrite)).Delete("/tasks", handler.Delete)
}

func (Module) Capabilities() []capability.Capability {
	return []capability.Capability{
		capability.TasksRead,
		capability.TasksWrite,
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "tasks_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	var payload deleteTasksRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if err := h.service.Delete(r.Context(), payload.IDs); err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "tasks_delete_failed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
