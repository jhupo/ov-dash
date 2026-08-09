package updates

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/updater"

	"github.com/go-chi/chi/v5"
)

const maxApplyRequestBytes = 4096

type Module struct{}

type Handler struct {
	service *Service
}

func NewModule() Module {
	return Module{}
}

func (Module) Manifest() platformmodule.Manifest {
	return platformmodule.Manifest{
		ID:          "updates",
		Title:       "Updates",
		Description: "Signed platform release checks and update operations.",
		Kind:        "platform",
		Tags:        []string{"platform", "deploy", "maintenance"},
	}
}

func (Module) Register(registrar *platformmodule.Registrar) error {
	if err := registrar.Capabilities(capability.UpdatesRead, capability.UpdatesApply); err != nil {
		return err
	}
	return registrar.HTTP(func(ctx platformmodule.Context) {
		handler := &Handler{service: NewService(ctx.Config.Update.SocketPath, nil)}
		readUpdates := ctx.RequireCapability(capability.UpdatesRead)
		applyUpdates := ctx.RequireCapability(capability.UpdatesApply)

		ctx.ProtectedRouter.With(readUpdates).Get("/updates", handler.Status)
		ctx.ProtectedRouter.With(readUpdates).Post("/updates/check", handler.Check)
		ctx.ProtectedRouter.With(applyUpdates).Post("/updates/apply", handler.Apply)
		ctx.ProtectedRouter.With(readUpdates).Get("/updates/operations/{id}", handler.Operation)
		ctx.ProtectedRouter.With(readUpdates).Get("/updates/operations/{id}/events", handler.OperationEvents)
	})
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Status(r.Context())
	if err != nil {
		writeServiceError(w, err, "update_status_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Check(r.Context())
	if err != nil {
		writeServiceError(w, err, "update_check_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ReleaseID string `json:"release_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxApplyRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || ensureJSONEOF(decoder) != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if err := updater.ValidateReleaseID(payload.ReleaseID); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_release_id"})
		return
	}
	result, err := h.service.Apply(r.Context(), payload.ReleaseID)
	if err != nil {
		writeServiceError(w, err, "update_apply_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, result)
}

func (h *Handler) Operation(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Operation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, ErrInvalidOperation) {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_operation_id"})
			return
		}
		writeServiceError(w, err, "update_operation_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) OperationEvents(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.OperationEvents(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, ErrInvalidOperation) {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_operation_id"})
			return
		}
		writeServiceError(w, err, "update_operation_events_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func writeServiceError(w http.ResponseWriter, err error, fallbackCode string) {
	status := http.StatusBadGateway
	code := fallbackCode
	var apiError *APIError
	if errors.As(err, &apiError) {
		switch apiError.StatusCode {
		case http.StatusBadRequest:
			status = http.StatusBadRequest
		case http.StatusNotFound:
			status = http.StatusNotFound
			code = "update_operation_not_found"
		case http.StatusConflict:
			status = http.StatusConflict
			code = "update_operation_conflict"
		}
	}
	httpx.WriteJSON(w, status, map[string]string{"error": code})
}

func ensureJSONEOF(decoder *json.Decoder) error {
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
