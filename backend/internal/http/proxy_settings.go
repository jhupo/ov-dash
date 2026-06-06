package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"ov-dash/backend/internal/modules/proxy"
)

type ProxySettingsHandler struct {
	service *proxy.Service
}

func NewProxySettingsHandler(service *proxy.Service) *ProxySettingsHandler {
	return &ProxySettingsHandler{service: service}
}

type updateProxySettingsRequest struct {
	Enabled       bool    `json:"enabled"`
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	Username      string  `json:"username"`
	Password      *string `json:"password"`
	ClearPassword bool    `json:"clear_password"`
}

func (h *ProxySettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	settings, err := h.service.Get(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "proxy_settings_get_failed"})
		return
	}
	writeJSON(w, http.StatusOK, settings.Public())
}

func (h *ProxySettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var payload updateProxySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	settings, err := h.service.Update(r.Context(), proxy.UpdateSettingsInput{
		Enabled:  payload.Enabled,
		Host:     payload.Host,
		Port:     payload.Port,
		Username: payload.Username,
		Password: payload.Password,
		ClearPassword: payload.ClearPassword,
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "proxy_settings_update_failed"
		if errors.Is(err, proxy.ErrHostRequired) || errors.Is(err, proxy.ErrInvalidPort) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		writeJSON(w, status, map[string]string{"error": code})
		return
	}

	writeJSON(w, http.StatusOK, settings.Public())
}
