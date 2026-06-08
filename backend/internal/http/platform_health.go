package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"ov-dash/backend/internal/platform"
	platformmodule "ov-dash/backend/internal/platform/module"
)

type platformHealthResponse struct {
	Status    string               `json:"status"`
	Items     []platformHealthItem `json:"items"`
	CheckedAt time.Time            `json:"checked_at"`
}

type platformHealthItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func platformHealthHandler(runtime *platform.Runtime, registry *platformmodule.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		checks := []platformmodule.HealthCheck{
			{ID: "database", Name: "Database", Check: func(ctx context.Context) error {
				return runtime.DB.Ping(ctx)
			}},
			{ID: "redis", Name: "Redis", Check: func(ctx context.Context) error {
				return runtime.Cache.Ping(ctx)
			}},
			{ID: "migrations", Name: "Migrations", Check: func(ctx context.Context) error {
				var failed int
				if err := runtime.DB.QueryRow(ctx, `
					SELECT count(*)
					FROM schema_migrations
					WHERE status = 'failed'
				`).Scan(&failed); err != nil {
					return err
				}
				if failed > 0 {
					return fmt.Errorf("%d failed migrations", failed)
				}
				return nil
			}},
			{ID: "worker", Name: "Worker heartbeat", Check: func(ctx context.Context) error {
				items, err := runtime.Queue.ListWorkerHeartbeats(ctx, 30*time.Second)
				if err != nil {
					return err
				}
				if len(items) == 0 {
					return fmt.Errorf("no worker heartbeat in the last 30s")
				}
				return nil
			}},
		}
		if registry != nil {
			checks = append(checks, registry.HealthChecks()...)
		}

		writeJSON(w, http.StatusOK, runPlatformHealthChecks(ctx, checks, time.Now().UTC()))
	}
}

func runPlatformHealthChecks(ctx context.Context, checks []platformmodule.HealthCheck, checkedAt time.Time) platformHealthResponse {
	response := platformHealthResponse{
		Status:    "ok",
		Items:     make([]platformHealthItem, 0, len(checks)),
		CheckedAt: checkedAt,
	}
	for _, check := range checks {
		item := platformHealthItem{
			ID:     check.ID,
			Name:   check.Name,
			Status: "ok",
		}
		if err := check.Check(ctx); err != nil {
			item.Status = "down"
			item.Message = err.Error()
			response.Status = "down"
		}
		response.Items = append(response.Items, item)
	}
	return response
}
