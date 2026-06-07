package proxy

import (
	"encoding/json"
	"errors"
	"net/http"

	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
)

type Module struct{}

type Handler struct {
	service *Service
}

type updateProxySettingsRequest struct {
	Enabled       bool    `json:"enabled"`
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	Username      string  `json:"username"`
	Password      *string `json:"password"`
	ClearPassword bool    `json:"clear_password"`
}

func NewModule() Module {
	return Module{}
}

func (Module) Name() string {
	return "proxy"
}

func (Module) RegisterRoutes(ctx platformmodule.Context) {
	handler := &Handler{service: NewService(NewRepository(ctx.DB))}
	ctx.ProtectedRouter.Get("/proxy-settings", handler.Get)
	ctx.ProtectedRouter.Put("/proxy-settings", handler.Update)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	settings, err := h.service.Get(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "proxy_settings_get_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, settings.Public())
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var payload updateProxySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	settings, err := h.service.Update(r.Context(), UpdateSettingsInput{
		Enabled:       payload.Enabled,
		Host:          payload.Host,
		Port:          payload.Port,
		Username:      payload.Username,
		Password:      payload.Password,
		ClearPassword: payload.ClearPassword,
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "proxy_settings_update_failed"
		if errors.Is(err, ErrHostRequired) || errors.Is(err, ErrInvalidPort) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		httpx.WriteJSON(w, status, map[string]string{"error": code})
		return
	}

	httpx.WriteJSON(w, http.StatusOK, settings.Public())
}
