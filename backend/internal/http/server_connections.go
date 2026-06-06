package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"ov-dash/backend/internal/modules/servers"

	"github.com/go-chi/chi/v5"
)

type ServerConnectionsHandler struct {
	service *servers.Service
}

func NewServerConnectionsHandler(service *servers.Service) *ServerConnectionsHandler {
	return &ServerConnectionsHandler{service: service}
}

type saveServerConnectionRequest struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	GroupName   string  `json:"group_name"`
	Region      string  `json:"region"`
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	Username    string  `json:"username"`
	AuthType    string  `json:"auth_type"`
	Password    *string `json:"password"`
	PrivateKey  *string `json:"private_key"`
	Passphrase  *string `json:"passphrase"`
	ClearSecret bool    `json:"clear_secret"`
}

func (h *ServerConnectionsHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_connections_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *ServerConnectionsHandler) Save(w http.ResponseWriter, r *http.Request) {
	var payload saveServerConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		payload.ID = id
	}

	item, err := h.service.Save(r.Context(), servers.SaveInput{
		ID:          payload.ID,
		Name:        payload.Name,
		GroupName:   payload.GroupName,
		Region:      payload.Region,
		Host:        payload.Host,
		Port:        payload.Port,
		Username:    payload.Username,
		AuthType:    payload.AuthType,
		Password:    payload.Password,
		PrivateKey:  payload.PrivateKey,
		Passphrase:  payload.Passphrase,
		ClearSecret: payload.ClearSecret,
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "server_connection_save_failed"
		if errors.Is(err, servers.ErrNameRequired) ||
			errors.Is(err, servers.ErrHostRequired) ||
			errors.Is(err, servers.ErrUsernameRequired) ||
			errors.Is(err, servers.ErrInvalidPort) ||
			errors.Is(err, servers.ErrInvalidAuthType) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		writeJSON(w, status, map[string]string{"error": code})
		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (h *ServerConnectionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_connection_delete_failed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
