package updater

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const maxRequestBytes = 4096

var operationIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type HTTPHandler struct {
	service *Service
}

func NewHTTPHandler(service *Service) (http.Handler, error) {
	if service == nil || service.controller == nil {
		return nil, errors.New("updater service is required")
	}
	return &HTTPHandler{service: service}, nil
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.RawQuery != "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "query parameters are not accepted")
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/status":
		h.status(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/check":
		h.check(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/operations":
		h.createOperation(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/operations/active":
		h.activeOperation(w)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/operations/"):
		h.getOperation(w, r)
	default:
		h.writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
	}
}

func (h *HTTPHandler) status(w http.ResponseWriter, r *http.Request) {
	current, err := h.service.Current(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "state_error", "cannot read installed release")
		return
	}
	active, err := h.service.ActiveOperation()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "state_error", "cannot read updater state")
		return
	}
	var operation any
	if active != nil {
		operation = operationView(*active)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"current": current, "operation": operation})
}

func (h *HTTPHandler) check(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "request body is not accepted")
		return
	}
	result, err := h.service.Check(r.Context())
	if err != nil {
		h.writeError(w, http.StatusBadGateway, "release_check_failed", err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, result)
}

func (h *HTTPHandler) createOperation(w http.ResponseWriter, r *http.Request) {
	if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		h.writeError(w, http.StatusUnsupportedMediaType, "invalid_content_type", "Content-Type must be application/json")
		return
	}
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxRequestBytes {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	var request struct {
		ReleaseID string `json:"release_id"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || requireJSONEOF(decoder) != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "request body must contain only release_id")
		return
	}
	operation, err := h.service.RequestUpdate(request.ReleaseID)
	if err != nil {
		switch {
		case errors.Is(err, ErrOperatorInterventionRequired):
			h.writeError(w, http.StatusConflict, "operator_intervention_required", err.Error())
		case errors.Is(err, ErrOperationActive):
			h.writeError(w, http.StatusConflict, "operation_active", err.Error())
		default:
			h.writeError(w, http.StatusBadRequest, "invalid_release", err.Error())
		}
		return
	}
	h.writeJSON(w, http.StatusAccepted, operationView(operation))
}

func (h *HTTPHandler) activeOperation(w http.ResponseWriter) {
	operation, err := h.service.ActiveOperation()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "state_error", "cannot read updater state")
		return
	}
	if operation == nil {
		h.writeJSON(w, http.StatusOK, map[string]any{"operation": nil})
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"operation": operationView(*operation)})
}

func (h *HTTPHandler) getOperation(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/operations/")
	if strings.HasSuffix(path, "/events") {
		id := strings.TrimSuffix(path, "/events")
		if !operationIDPattern.MatchString(id) {
			h.writeError(w, http.StatusNotFound, "not_found", "operation not found")
			return
		}
		events, err := h.service.OperationEvents(id)
		if err != nil {
			h.writeStoreError(w, err)
			return
		}
		views := make([]eventView, 0, len(events))
		for _, event := range events {
			views = append(views, eventView{
				Revision: event.Revision, Previous: event.Previous, State: event.State,
				RecordedAt: event.RecordedAt, Error: event.Operation.LastError,
			})
		}
		h.writeJSON(w, http.StatusOK, map[string]any{"items": views})
		return
	}
	if !operationIDPattern.MatchString(path) {
		h.writeError(w, http.StatusNotFound, "not_found", "operation not found")
		return
	}
	operation, err := h.service.Operation(path)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, operationView(operation))
}

func (h *HTTPHandler) writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrOperationNotFound) {
		h.writeError(w, http.StatusNotFound, "not_found", "operation not found")
		return
	}
	h.writeError(w, http.StatusInternalServerError, "state_error", "cannot read updater state")
}

func (h *HTTPHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func (h *HTTPHandler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type operationResponse struct {
	ID             string            `json:"id"`
	ReleaseID      string            `json:"release_id"`
	State          State             `json:"state"`
	Revision       uint64            `json:"revision"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Manifest       *ReleaseManifest  `json:"manifest,omitempty"`
	Previous       *InstalledRelease `json:"previous,omitempty"`
	Backup         *Backup           `json:"backup,omitempty"`
	LastError      string            `json:"last_error,omitempty"`
	RecoveryReason string            `json:"recovery_reason,omitempty"`
}

func operationView(operation Operation) operationResponse {
	return operationResponse{
		ID: operation.ID, ReleaseID: operation.ReleaseID, State: operation.State, Revision: operation.Revision,
		CreatedAt: operation.CreatedAt, UpdatedAt: operation.UpdatedAt, Manifest: operation.Manifest,
		Previous: operation.Previous, Backup: operation.Backup, LastError: operation.LastError,
		RecoveryReason: operation.RecoveryReason,
	}
}

type eventView struct {
	Revision   uint64    `json:"revision"`
	Previous   State     `json:"previous,omitempty"`
	State      State     `json:"state"`
	RecordedAt time.Time `json:"recorded_at"`
	Error      string    `json:"error,omitempty"`
}
