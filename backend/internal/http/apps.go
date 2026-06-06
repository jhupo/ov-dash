package http

import (
	"net/http"

	"ov-dash/backend/internal/modules/apps"
)

type AppsHandler struct {
	service *apps.Service
}

func NewAppsHandler(service *apps.Service) *AppsHandler {
	return &AppsHandler{service: service}
}

func (h *AppsHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "apps_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
