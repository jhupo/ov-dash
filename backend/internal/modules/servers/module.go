package servers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/ssh"
)

type Module struct{}

type Handler struct {
	service   *Service
	collector *Collector
	cache     cacheStore
}

type cacheStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
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

type serverCommandRequest struct {
	Command string `json:"command"`
}

func NewModule() Module {
	return Module{}
}

func (Module) ID() string {
	return "servers"
}

func (Module) RegisterHTTP(ctx platformmodule.Context) {
	repository := NewRepositoryWithSecrets(ctx.DB, ctx.Secrets)
	handler := &Handler{
		service:   NewService(repository),
		collector: NewCollector(repository),
		cache:     ctx.Cache,
	}
	readServers := ctx.RequireCapability(capability.ServersRead)
	writeServers := ctx.RequireCapability(capability.ServersWrite)
	deleteServers := ctx.RequireCapability(capability.ServersDelete)
	sshServers := ctx.RequireCapability(capability.ServersSSH)

	ctx.ProtectedRouter.With(readServers).Get("/server-connections", handler.List)
	ctx.ProtectedRouter.With(readServers).Get("/server-connections/{id}/metrics", handler.Metrics)
	ctx.ProtectedRouter.With(writeServers).Post("/server-connections/monitor/touch", handler.TouchMonitor)
	ctx.ProtectedRouter.With(writeServers).Post("/server-connections", handler.Save)
	ctx.ProtectedRouter.With(writeServers).Put("/server-connections/{id}", handler.Save)
	ctx.ProtectedRouter.With(writeServers).Post("/server-connections/{id}/agent/update", handler.UpdateAgent)
	ctx.ProtectedRouter.With(sshServers).Post("/server-connections/{id}/ssh/command", handler.RunCommand)
	ctx.ProtectedRouter.With(sshServers).Post("/server-connections/{id}/ssh/ticket", handler.IssueShellTicket)
	ctx.PublicRouter.Get("/server-connections/{id}/ssh/ws", handler.Shell)
	ctx.ProtectedRouter.With(deleteServers).Delete("/server-connections/{id}", handler.Delete)
}

func (Module) Capabilities() []capability.Capability {
	return []capability.Capability{
		capability.ServersRead,
		capability.ServersWrite,
		capability.ServersDelete,
		capability.ServersSSH,
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_connections_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	var payload saveServerConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		payload.ID = id
	}
	expiresAt, err := parseOptionalTime(payload.ExpiresAt)
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_expires_at"})
		return
	}

	item, err := h.service.Save(r.Context(), SaveInput{
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
		if errors.Is(err, ErrNameRequired) ||
			errors.Is(err, ErrHostRequired) ||
			errors.Is(err, ErrUsernameRequired) ||
			errors.Is(err, ErrInvalidPort) ||
			errors.Is(err, ErrInvalidAuthType) {
			status = http.StatusBadRequest
			code = err.Error()
		}
		httpx.WriteJSON(w, status, map[string]string{"error": code})
		return
	}

	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
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
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_metrics_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) TouchMonitor(w http.ResponseWriter, r *http.Request) {
	if err := h.collector.TouchMonitor(r.Context(), 30*time.Second); err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_monitor_touch_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := h.collector.Install(ctx, chi.URLParam(r, "id")); err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) RunCommand(w http.ResponseWriter, r *http.Request) {
	var payload serverCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	command := strings.TrimSpace(payload.Command)
	if command == "" || len(command) > 2000 {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_command"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	output, err := h.collector.RunCommand(ctx, chi.URLParam(r, "id"), command)
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  err.Error(),
			"output": output,
		})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"output": output})
}

func (h *Handler) IssueShellTicket(w http.ResponseWriter, r *http.Request) {
	if h.cache == nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "ssh_ticket_store_unavailable"})
		return
	}
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_server_id"})
		return
	}
	ticket, err := randomTicket()
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "ssh_ticket_create_failed"})
		return
	}
	expiresIn := 30 * time.Second
	if err := h.cache.Set(r.Context(), shellTicketKey(ticket), serverID, expiresIn); err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "ssh_ticket_store_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ticket":     ticket,
		"expires_at": time.Now().Add(expiresIn),
	})
}

func (h *Handler) Shell(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if err := h.consumeShellTicket(r.Context(), serverID, r.URL.Query().Get("ticket")); err != nil {
		httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	ws, err := upgradeWebSocket(w, r)
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "websocket_upgrade_failed"})
		return
	}
	defer ws.close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	client, session, err := h.collector.Shell(ctx, serverID)
	if err != nil {
		_ = ws.writeText("connect failed: " + err.Error() + "\r\n")
		return
	}
	defer client.Close()
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = ws.writeText("open stdin failed: " + err.Error() + "\r\n")
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = ws.writeText("open stdout failed: " + err.Error() + "\r\n")
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = ws.writeText("open stderr failed: " + err.Error() + "\r\n")
		return
	}
	if err := session.Shell(); err != nil {
		_ = ws.writeText("start shell failed: " + err.Error() + "\r\n")
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
			if handled, err := handleTerminalResize(session, text); handled {
				if err != nil {
					_ = ws.writeText("resize failed: " + err.Error() + "\r\n")
				}
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

func (h *Handler) consumeShellTicket(ctx context.Context, serverID string, ticket string) error {
	if h.cache == nil {
		return errors.New("ssh_ticket_store_unavailable")
	}
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return errors.New("missing_ssh_ticket")
	}
	key := shellTicketKey(ticket)
	storedServerID, err := h.cache.Get(ctx, key)
	if err != nil {
		return errors.New("invalid_ssh_ticket")
	}
	_ = h.cache.Delete(ctx, key)
	if storedServerID != serverID {
		return errors.New("invalid_ssh_ticket")
	}
	return nil
}

func shellTicketKey(ticket string) string {
	return "servers:ssh-ticket:" + ticket
}

func randomTicket() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func handleTerminalResize(session *ssh.Session, text string) (bool, error) {
	const prefix = "\x1b]ovdash-resize;"
	const suffix = "\x07"
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, suffix) {
		return false, nil
	}
	size := strings.TrimSuffix(strings.TrimPrefix(text, prefix), suffix)
	parts := strings.Split(size, ";")
	if len(parts) != 2 {
		return true, nil
	}
	cols, err := strconv.Atoi(parts[0])
	if err != nil {
		return true, nil
	}
	rows, err := strconv.Atoi(parts[1])
	if err != nil {
		return true, nil
	}
	if cols < 20 || rows < 5 || cols > 300 || rows > 120 {
		return true, nil
	}
	return true, session.WindowChange(rows, cols)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_connection_delete_failed"})
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
