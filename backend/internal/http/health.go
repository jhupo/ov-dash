package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"ov-dash/backend/internal/cache"
	"ov-dash/backend/internal/database"
	"ov-dash/backend/internal/db"
	"ov-dash/backend/internal/queue"
)

type HealthHandler struct {
	releaseID            string
	pingPostgres         func(context.Context) error
	pingRedis            func(context.Context) error
	verifyMigrations     func(context.Context) error
	listWorkerHeartbeats func(context.Context, time.Duration) ([]queue.WorkerHeartbeat, error)
}

func NewHealthHandler(db *db.Pool, cache *cache.Cache, queueClient *queue.Client, migrationsDir string, version string) *HealthHandler {
	handler := &HealthHandler{
		releaseID: queue.NormalizeReleaseID(version),
		pingPostgres: func(ctx context.Context) error {
			if db == nil {
				return errors.New("postgres is not configured")
			}
			return db.Ping(ctx)
		},
		pingRedis: func(ctx context.Context) error {
			if cache == nil {
				return errors.New("redis is not configured")
			}
			return cache.Ping(ctx)
		},
		verifyMigrations: func(ctx context.Context) error {
			if db == nil {
				return errors.New("postgres is not configured")
			}
			return database.NewMigrationRunner(db).VerifyDir(ctx, migrationsDir)
		},
	}
	if queueClient != nil {
		handler.listWorkerHeartbeats = queueClient.ListWorkerHeartbeats
	}
	return handler
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, _ *http.Request) {
	h.writeReleaseHeader(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "release_id": h.releaseID})
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	checks := map[string]string{
		"postgres":   "ok",
		"redis":      "ok",
		"migrations": "ok",
		"worker":     "ok",
	}
	status := http.StatusOK

	if err := h.pingPostgres(ctx); err != nil {
		checks["postgres"] = "down"
		status = http.StatusServiceUnavailable
	}
	if err := h.pingRedis(ctx); err != nil {
		checks["redis"] = "down"
		status = http.StatusServiceUnavailable
	}
	if err := h.verifyMigrations(ctx); err != nil {
		checks["migrations"] = "down"
		status = http.StatusServiceUnavailable
	}
	if r.URL.Query().Get("scope") == "api" {
		delete(checks, "worker")
	} else {
		if h.listWorkerHeartbeats == nil {
			checks["worker"] = "down"
			status = http.StatusServiceUnavailable
		} else if workers, err := h.listWorkerHeartbeats(ctx, 30*time.Second); err != nil || !hasWorkerHeartbeatForRelease(workers, h.releaseID) {
			checks["worker"] = "down"
			status = http.StatusServiceUnavailable
		}
	}

	h.writeReleaseHeader(w)
	writeJSON(w, status, map[string]any{
		"status":     statusText(status),
		"release_id": h.releaseID,
		"checks":     checks,
	})
}

func (h *HealthHandler) writeReleaseHeader(w http.ResponseWriter) {
	if h.releaseID != "" {
		w.Header().Set("X-OV-Dash-Release", h.releaseID)
	}
}

func releaseID(version string) string {
	return queue.NormalizeReleaseID(version)
}

func hasWorkerHeartbeatForRelease(items []queue.WorkerHeartbeat, releaseID string) bool {
	for _, item := range items {
		if item.ReleaseID == releaseID {
			return true
		}
	}
	return false
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
