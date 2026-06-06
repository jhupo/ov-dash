package http

import (
	"net/http"

	"ov-dash/backend/internal/modules/dashboard"
)

type DashboardHandler struct {
	service *dashboard.Service
}

func NewDashboardHandler(service *dashboard.Service) *DashboardHandler {
	return &DashboardHandler{service: service}
}

func (h *DashboardHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.service.Snapshot(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dashboard_snapshot_failed"})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}
