package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"ov-dash/backend/internal/modules/servers"

	"github.com/go-chi/chi/v5"
)

type ServerConnectionsHandler struct {
	service   *servers.Service
	collector *servers.Collector
}

func NewServerConnectionsHandler(service *servers.Service, collector *servers.Collector) *ServerConnectionsHandler {
	return &ServerConnectionsHandler{service: service, collector: collector}
}

type saveServerConnectionRequest struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	GroupName       string  `json:"group_name"`
	Region          string  `json:"region"`
	Host            string  `json:"host"`
	Port            int     `json:"port"`
	Username        string  `json:"username"`
	AuthType        string  `json:"auth_type"`
	Password        *string `json:"password"`
	PrivateKey      *string `json:"private_key"`
	ExpiresAt       *string `json:"expires_at"`
	CollectInterval int     `json:"collect_interval_seconds"`
	ClearSecret     bool    `json:"clear_secret"`
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
	expiresAt, err := parseOptionalTime(payload.ExpiresAt)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_expires_at"})
		return
	}

	item, err := h.service.Save(r.Context(), servers.SaveInput{
		ID:              payload.ID,
		Name:            payload.Name,
		GroupName:       payload.GroupName,
		Region:          payload.Region,
		Host:            payload.Host,
		Port:            payload.Port,
		Username:        payload.Username,
		AuthType:        payload.AuthType,
		Password:        payload.Password,
		PrivateKey:      payload.PrivateKey,
		ExpiresAt:       expiresAt,
		CollectInterval: payload.CollectInterval,
		ClearSecret:     payload.ClearSecret,
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

func (h *ServerConnectionsHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-time.Hour)
	switch strings.TrimSpace(r.URL.Query().Get("range")) {
	case "4h":
		since = time.Now().Add(-4 * time.Hour)
	case "1d":
		since = time.Now().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	}
	items, err := h.service.Samples(r.Context(), chi.URLParam(r, "id"), since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_metrics_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *ServerConnectionsHandler) TouchMonitor(w http.ResponseWriter, r *http.Request) {
	if err := h.collector.TouchMonitor(r.Context(), 30*time.Second); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_monitor_touch_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type serverCommandRequest struct {
	Command string `json:"command"`
}

func (h *ServerConnectionsHandler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := h.collector.Install(ctx, chi.URLParam(r, "id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *ServerConnectionsHandler) RunCommand(w http.ResponseWriter, r *http.Request) {
	var payload serverCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	command := strings.TrimSpace(payload.Command)
	if command == "" || len(command) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_command"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	output, err := h.collector.RunCommand(ctx, chi.URLParam(r, "id"), command)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  err.Error(),
			"output": output,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": output})
}

func (h *ServerConnectionsHandler) Shell(w http.ResponseWriter, r *http.Request) {
	ws, err := upgradeWebSocket(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "websocket_upgrade_failed"})
		return
	}
	defer ws.close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	client, session, err := h.collector.Shell(ctx, chi.URLParam(r, "id"))
	if err != nil {
		_ = ws.writeText("连接失败: " + err.Error() + "\r\n")
		return
	}
	defer client.Close()
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = ws.writeText("打开输入失败: " + err.Error() + "\r\n")
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = ws.writeText("打开输出失败: " + err.Error() + "\r\n")
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = ws.writeText("打开错误输出失败: " + err.Error() + "\r\n")
		return
	}
	if err := session.Shell(); err != nil {
		_ = ws.writeText("启动 Shell 失败: " + err.Error() + "\r\n")
		return
	}

	done := make(chan struct{})
	stream := func(reader io.Reader) {
		buffer := make([]byte, 4096)
		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				if writeErr := ws.writeText(string(buffer[:n])); writeErr != nil {
					cancel()
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	go stream(stdout)
	go stream(stderr)
	go func() {
		defer close(done)
		for {
			text, err := ws.readText()
			if err != nil {
				cancel()
				return
			}
			if text == "" {
				continue
			}
			if _, err := io.WriteString(stdin, text); err != nil {
				cancel()
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

func (h *ServerConnectionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_connection_delete_failed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseOptionalTime(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return &parsed, nil
		}
	}
	return nil, errors.New("invalid time")
}
