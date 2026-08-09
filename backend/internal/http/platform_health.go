package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"ov-dash/backend/internal/database"
	"ov-dash/backend/internal/platform"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/queue"
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

func platformHealthHandler(runtime *platform.Runtime, catalog *platformmodule.Catalog) http.HandlerFunc {
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
				return database.NewMigrationRunner(runtime.DB).VerifyDir(ctx, runtime.Config.Migrations.Dir)
			}},
			{ID: "worker", Name: "Worker heartbeat", Check: func(ctx context.Context) error {
				items, err := runtime.Queue.ListWorkerHeartbeats(ctx, 30*time.Second)
				if err != nil {
					return err
				}
				releaseID := queue.NormalizeReleaseID(runtime.Config.App.Version)
				if !hasWorkerHeartbeatForRelease(items, releaseID) {
					return fmt.Errorf("no worker heartbeat for release %s in the last 30s", releaseID)
				}
				return nil
			}},
		}
		if catalog != nil {
			checks = append(checks, catalog.HealthChecks()...)
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
