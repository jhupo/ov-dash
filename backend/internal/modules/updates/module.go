package updates

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
	platformsettings "ov-dash/backend/internal/platform/settings"
	"ov-dash/backend/internal/updater"

	"github.com/go-chi/chi/v5"
)

const maxApplyRequestBytes = 4096

type Module struct{}

type Handler struct {
	service *updater.Service
	initErr error
}

func NewModule() Module {
	return Module{}
}

func (Module) Manifest() platformmodule.Manifest {
	return platformmodule.Manifest{
		ID:          "updates",
		Title:       "Updates",
		Description: "Signed application package updates.",
		Kind:        "platform",
		Tags:        []string{"platform", "deploy", "maintenance"},
	}
}

func (Module) Register(registrar *platformmodule.Registrar) error {
	if err := registrar.Capabilities(capability.UpdatesRead, capability.UpdatesApply); err != nil {
		return err
	}
	proxyURLSetting := platformsettings.Schema{
		Key:      "update.proxy_url",
		Type:     platformsettings.TypeString,
		Default:  registrar.Context().Config.Update.ProxyURL,
		Writable: true,
		Validation: platformsettings.Validation{
			MaxLength: intPointer(2048),
			Pattern:   `^$|^https://[^\s]+$`,
		},
	}
	if err := registrar.Settings(proxyURLSetting); err != nil {
		return err
	}
	return registrar.HTTP(func(ctx platformmodule.Context) {
		settingsStore := platformsettings.NewStore(ctx.DB, ctx.Secrets)
		resolveProxy := func(requestContext context.Context) (string, error) {
			value, err := settingsStore.Get(requestContext, proxyURLSetting)
			if err != nil {
				return "", err
			}
			proxyURL, _ := value.Value.(string)
			proxyURL = strings.TrimSpace(proxyURL)
			if proxyURL == "" {
				proxyURL = ctx.Config.Update.ProxyURL
			}
			return strings.TrimSpace(proxyURL), nil
		}
		service, err := updater.NewService(updater.ServiceConfig{
			RuntimeDir:        ctx.Config.Update.RuntimeDir,
			ReleaseRepository: ctx.Config.Update.ReleaseRepository,
			PublicKey:         ctx.Config.Update.PublicKey,
			ExitDelay:         ctx.Config.Update.ExitDelay,
			ResolveProxy:      resolveProxy,
			RequestShutdown:   ctx.RequestShutdown,
		})
		handler := &Handler{service: service, initErr: err}
		readUpdates := ctx.RequireCapability(capability.UpdatesRead)
		applyUpdates := ctx.RequireCapability(capability.UpdatesApply)

		ctx.ProtectedRouter.With(readUpdates).Get("/updates", handler.Status)
		ctx.ProtectedRouter.With(readUpdates).Post("/updates/check", handler.Check)
		ctx.ProtectedRouter.With(applyUpdates).Post("/updates/apply", handler.Apply)
		ctx.ProtectedRouter.With(readUpdates).Get("/updates/operations/{id}", handler.Operation)
	})
}

func (h *Handler) Status(w http.ResponseWriter, _ *http.Request) {
	if !h.available(w) {
		return
	}
	result, err := h.service.Status()
	if err != nil {
		writeServiceError(w, err, "update_status_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	result, err := h.service.Check(r.Context())
	if err != nil {
		writeServiceError(w, err, "update_check_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	var payload struct {
		ReleaseID string `json:"release_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxApplyRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || ensureJSONEOF(decoder) != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
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
	if !h.available(w) {
		return
	}
	result, err := h.service.Operation(chi.URLParam(r, "id"))
	if err != nil {
		writeServiceError(w, err, "update_operation_failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) available(w http.ResponseWriter) bool {
	if h.initErr == nil && h.service != nil {
		return true
	}
	httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "update_service_unavailable"})
	return false
}

func writeServiceError(w http.ResponseWriter, err error, fallbackCode string) {
	status := http.StatusBadGateway
	code := fallbackCode
	switch {
	case errors.Is(err, updater.ErrOperationActive), errors.Is(err, updater.ErrReleaseMismatch):
		status = http.StatusConflict
	case errors.Is(err, updater.ErrOperationNotFound):
		status = http.StatusNotFound
		code = "update_operation_not_found"
	}
	httpx.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
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

func intPointer(value int) *int {
	return &value
}
