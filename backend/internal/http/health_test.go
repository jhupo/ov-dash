package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ov-dash/backend/internal/queue"
)

func TestReadinessRequiresWorkerFromCurrentRelease(t *testing.T) {
	tests := []struct {
		name       string
		heartbeats []queue.WorkerHeartbeat
		wantStatus int
		wantWorker string
	}{
		{
			name:       "matching release",
			heartbeats: []queue.WorkerHeartbeat{{ReleaseID: "ov-dash-1.2.3"}},
			wantStatus: http.StatusOK,
			wantWorker: "ok",
		},
		{
			name:       "different release",
			heartbeats: []queue.WorkerHeartbeat{{ReleaseID: "ov-dash-1.2.2"}},
			wantStatus: http.StatusServiceUnavailable,
			wantWorker: "down",
		},
		{
			name: "exact match among workers",
			heartbeats: []queue.WorkerHeartbeat{
				{ReleaseID: "ov-dash-1.2.2"},
				{ReleaseID: "ov-dash-1.2.3"},
			},
			wantStatus: http.StatusOK,
			wantWorker: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := healthyTestHealthHandler("v1.2.3", tt.heartbeats)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/readyz", nil)

			handler.Readiness(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.wantStatus, response.Body.String())
			}
			if got := response.Header().Get("X-OV-Dash-Release"); got != "ov-dash-1.2.3" {
				t.Fatalf("release header = %q", got)
			}
			var body struct {
				Checks map[string]string `json:"checks"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got := body.Checks["worker"]; got != tt.wantWorker {
				t.Fatalf("worker check = %q, want %q", got, tt.wantWorker)
			}
		})
	}
}

func TestReadinessAPIScopeSkipsWorkerHeartbeat(t *testing.T) {
	workerChecked := false
	handler := healthyTestHealthHandler("1.2.3", nil)
	handler.listWorkerHeartbeats = func(context.Context, time.Duration) ([]queue.WorkerHeartbeat, error) {
		workerChecked = true
		return nil, errors.New("worker check must be skipped")
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/readyz?scope=api", nil)

	handler.Readiness(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if workerChecked {
		t.Fatal("scope=api called worker heartbeat check")
	}
	var body struct {
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, exists := body.Checks["worker"]; exists {
		t.Fatalf("scope=api response contains worker check: %#v", body.Checks)
	}
}

func TestWorkerHeartbeatReleaseMatchIsExact(t *testing.T) {
	items := []queue.WorkerHeartbeat{{ReleaseID: "ov-dash-1.2.30"}}
	if hasWorkerHeartbeatForRelease(items, "ov-dash-1.2.3") {
		t.Fatal("release match accepted a prefix")
	}
}

func healthyTestHealthHandler(version string, heartbeats []queue.WorkerHeartbeat) *HealthHandler {
	return &HealthHandler{
		releaseID: queue.NormalizeReleaseID(version),
		pingPostgres: func(context.Context) error {
			return nil
		},
		pingRedis: func(context.Context) error {
			return nil
		},
		verifyMigrations: func(context.Context) error {
			return nil
		},
		listWorkerHeartbeats: func(context.Context, time.Duration) ([]queue.WorkerHeartbeat, error) {
			return heartbeats, nil
		},
	}
}
