package updates

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestServiceUsesFixedUpdaterEndpoints(t *testing.T) {
	operationID := strings.Repeat("a", 32)
	tests := []struct {
		name        string
		method      string
		path        string
		response    string
		call        func(*Service) error
		requestBody string
	}{
		{name: "health", method: http.MethodGet, path: "/healthz", response: `{"status":"ok"}`, call: func(service *Service) error { _, err := service.Health(context.Background()); return err }},
		{name: "status", method: http.MethodGet, path: "/v1/status", response: `{"current":{"release_id":"ov-dash-1.0.0","version":"1.0.0","sequence":1,"schema":1,"images":{},"committed_at":"2026-08-09T00:00:00Z"},"operation":null}`, call: func(service *Service) error { _, err := service.Status(context.Background()); return err }},
		{name: "check", method: http.MethodPost, path: "/v1/check", response: `{"current":{"release_id":"ov-dash-1.0.0","version":"1.0.0","sequence":1,"schema":1,"images":{},"committed_at":"2026-08-09T00:00:00Z"},"candidate":{"schema_version":1,"release_id":"ov-dash-1.1.0","version":"1.1.0","sequence":2,"published_at":"2026-08-09T00:00:00Z","expires_at":"2026-08-10T00:00:00Z","minimum_version":"1.0.0","images":{},"database":{"from_schema":1,"to_schema":1,"strategy":"snapshot","transactional":true,"backup_required":true},"health":{"timeout_seconds":30,"stability_seconds":5}},"has_update":true}`, call: func(service *Service) error { _, err := service.Check(context.Background()); return err }},
		{name: "apply", method: http.MethodPost, path: "/v1/operations", response: `{"id":"` + operationID + `","release_id":"ov-dash-1.1.0","state":"requested","revision":1,"created_at":"2026-08-09T00:00:00Z","updated_at":"2026-08-09T00:00:00Z"}`, requestBody: `{"release_id":"ov-dash-1.1.0"}`, call: func(service *Service) error {
			_, err := service.Apply(context.Background(), "ov-dash-1.1.0")
			return err
		}},
		{name: "operation", method: http.MethodGet, path: "/v1/operations/" + operationID, response: `{"id":"` + operationID + `","release_id":"ov-dash-1.1.0","state":"requested","revision":1,"created_at":"2026-08-09T00:00:00Z","updated_at":"2026-08-09T00:00:00Z"}`, call: func(service *Service) error {
			_, err := service.Operation(context.Background(), operationID)
			return err
		}},
		{name: "events", method: http.MethodGet, path: "/v1/operations/" + operationID + "/events", response: `{"items":[{"revision":1,"state":"requested","recorded_at":"2026-08-09T00:00:00Z"}]}`, call: func(service *Service) error {
			_, err := service.OperationEvents(context.Background(), operationID)
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != test.method || request.URL.String() != updaterBaseURL+test.path || request.URL.RawQuery != "" {
					t.Fatalf("request = %s %s", request.Method, request.URL.String())
				}
				var body []byte
				if request.Body != nil {
					var err error
					body, err = io.ReadAll(request.Body)
					if err != nil {
						t.Fatal(err)
					}
				}
				if string(body) != test.requestBody {
					t.Fatalf("body = %q, want %q", string(body), test.requestBody)
				}
				return jsonResponse(http.StatusOK, test.response), nil
			})}
			if err := test.call(NewService("/run/ov-dash/updater.sock", client)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServiceRejectsUntrustedIdentifiersBeforeRequest(t *testing.T) {
	called := false
	service := NewService("/run/ov-dash/updater.sock", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected request")
	})})

	if _, err := service.Apply(context.Background(), "../../shell"); err == nil {
		t.Fatal("Apply accepted an invalid release_id")
	}
	if _, err := service.Operation(context.Background(), "../active"); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("Operation error = %v", err)
	}
	if _, err := service.OperationEvents(context.Background(), strings.Repeat("g", 32)); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("OperationEvents error = %v", err)
	}
	if called {
		t.Fatal("invalid identifier reached HTTP transport")
	}
}

func TestServiceReturnsStructuredAPIError(t *testing.T) {
	service := NewService("/run/ov-dash/updater.sock", responseClient(http.StatusConflict, `{"error":{"code":"operation_active","message":"an operation is already active"}}`))
	_, err := service.Apply(context.Background(), "ov-dash-1.1.0")
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %T %v", err, err)
	}
	if apiError.StatusCode != http.StatusConflict || apiError.Code != "operation_active" || apiError.Message == "" {
		t.Fatalf("APIError = %#v", apiError)
	}
}

func TestServiceRejectsMalformedAndOversizedResponses(t *testing.T) {
	t.Run("unknown field", func(t *testing.T) {
		service := NewService("/run/ov-dash/updater.sock", responseClient(http.StatusOK, `{"status":"ok","extra":true}`))
		if _, err := service.Health(context.Background()); err == nil {
			t.Fatal("Health accepted an unknown response field")
		}
	})

	t.Run("oversized", func(t *testing.T) {
		service := NewService("/run/ov-dash/updater.sock", responseClient(http.StatusOK, strings.Repeat("x", maxResponseBytes+1)))
		_, err := service.Health(context.Background())
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("malformed error", func(t *testing.T) {
		service := NewService("/run/ov-dash/updater.sock", responseClient(http.StatusBadGateway, `{"error":"bad"}`))
		_, err := service.Check(context.Background())
		var apiError *APIError
		if err == nil || errors.As(err, &apiError) {
			t.Fatalf("error = %T %v", err, err)
		}
	})
}

func TestServiceEnforcesRequestDeadline(t *testing.T) {
	service := NewService("/run/ov-dash/updater.sock", &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := service.Health(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
}

func TestServiceRequiresSocketPath(t *testing.T) {
	service := NewService(" ", responseClient(http.StatusOK, `{"status":"ok"}`))
	if _, err := service.Health(context.Background()); !errors.Is(err, ErrInvalidSocketPath) {
		t.Fatalf("error = %v", err)
	}
}

func responseClient(status int, body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(status, body), nil
	})}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
