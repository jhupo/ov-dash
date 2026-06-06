package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/queue"
)

type HealthHandler struct {
	db    *db.Pool
	queue *queue.Client
}

func NewHealthHandler(db *db.Pool, queue *queue.Client) *HealthHandler {
	return &HealthHandler{db: db, queue: queue}
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	checks := map[string]string{"postgres": "ok", "redis": "ok"}
	status := http.StatusOK

	if err := h.db.Ping(ctx); err != nil {
		checks["postgres"] = "down"
		status = http.StatusServiceUnavailable
	}
	if err := h.queue.Ping(ctx); err != nil {
		checks["redis"] = "down"
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, map[string]any{
		"status": statusText(status),
		"checks": checks,
	})
}

func statusText(status int) string {
	if status >= 200 && status < 300 {
		return "ok"
	}
	return "degraded"
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
