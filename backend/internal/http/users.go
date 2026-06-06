package http

import (
	"net/http"

	"ov-dash/backend/internal/modules/users"
)

type UsersHandler struct {
	service *users.Service
}

func NewUsersHandler(service *users.Service) *UsersHandler {
	return &UsersHandler{service: service}
}

func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "users_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
