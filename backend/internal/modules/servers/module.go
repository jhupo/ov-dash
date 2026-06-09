package servers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/platform/audit"
	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/platform/redact"
	"ov-dash/backend/internal/queue"

	"github.com/go-chi/chi/v5"
)

type Module struct{}

type Handler struct {
	service   *Service
	collector *Collector
	queue     serverJobQueue
	queueName string
	activity  agentActivityQueue
	cache     cacheStore
	audit     audit.RequestRecorder
}

type serverJobQueue interface {
	Enqueue(ctx context.Context, queueName string, job queue.Job) error
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
		queue:     ctx.Queue,
		queueName: ctx.Config.Worker.QueueName,
		activity:  ctx.Queue,
		cache:     ctx.Cache,
		audit:     ctx.Audit,
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
	ctx.ProtectedRouter.With(readServers).Get("/server-connections/{id}/agent/status", handler.AgentStatus)
	ctx.ProtectedRouter.With(readServers).Get("/server-connections/{id}/agent/diagnostics", handler.AgentDiagnostics)
	ctx.ProtectedRouter.With(readServers).Get("/server-connections/{id}/agent/activity", handler.AgentActivity)
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
	serverID := chi.URLParam(r, "id")
	if h.queue == nil {
		h.recordAudit(r, "servers.agent.update", serverID, "failure", "job_queue_unavailable", nil)
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "job_queue_unavailable"})
		return
	}
	job, err := NewAgentUpdateJob(serverID)
	if err != nil {
		h.recordAudit(r, "servers.agent.update", serverID, "failure", err.Error(), nil)
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.queue.Enqueue(r.Context(), h.queueName, job); err != nil {
		h.recordAudit(r, "servers.agent.update", serverID, "failure", err.Error(), nil)
		if errors.Is(err, queue.ErrDuplicateIdempotencyKey) {
			httpx.WriteJSON(w, http.StatusConflict, map[string]string{"error": "duplicate_agent_update_job"})
			return
		}
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent_update_enqueue_failed"})
		return
	}
	h.recordAudit(r, "servers.agent.update", serverID, "success", "", map[string]any{
		"job_id":          job.ID,
		"job_type":        job.Type,
		"idempotency_key": job.IdempotencyKey,
	})
	httpx.WriteJSON(w, http.StatusAccepted, job)
}

func (h *Handler) AgentStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	status, err := h.collector.AgentStatus(ctx, chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) AgentDiagnostics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	diagnostics, err := h.collector.AgentDiagnostics(ctx, chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, diagnostics)
}

func (h *Handler) AgentActivity(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "server_connection_not_found"})
		return
	}
	activity, err := BuildAgentActivity(r.Context(), item, h.activity)
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_agent_activity_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, activity)
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
	serverID := chi.URLParam(r, "id")
	metadata := map[string]any{
		"command":        redact.Text(command),
		"command_length": len(command),
	}
	output, err := h.collector.RunCommand(ctx, serverID, command)
	if err != nil {
		h.recordAudit(r, "servers.ssh.command", serverID, "failure", err.Error(), metadata)
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  err.Error(),
			"output": output,
		})
		return
	}
	h.recordAudit(r, "servers.ssh.command", serverID, "success", "", metadata)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"output": output})
}

func (h *Handler) IssueShellTicket(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if h.cache == nil {
		h.recordAudit(r, "servers.ssh.ticket", serverID, "failure", "ssh_ticket_store_unavailable", nil)
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "ssh_ticket_store_unavailable"})
		return
	}
	if serverID == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_server_id"})
		return
	}
	ticket, err := randomTicket()
	if err != nil {
		h.recordAudit(r, "servers.ssh.ticket", serverID, "failure", err.Error(), nil)
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "ssh_ticket_create_failed"})
		return
	}
	expiresIn, expiresAt := shellTicketExpiry(time.Now())
	if err := h.cache.Set(r.Context(), shellTicketKey(ticket), serverID, expiresIn); err != nil {
		h.recordAudit(r, "servers.ssh.ticket", serverID, "failure", err.Error(), nil)
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "ssh_ticket_store_failed"})
		return
	}
	h.recordAudit(r, "servers.ssh.ticket", serverID, "success", "", map[string]any{"expires_at": expiresAt})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ticket":     ticket,
		"expires_at": expiresAt,
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

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if err := h.service.Delete(r.Context(), serverID); err != nil {
		h.recordAudit(r, "servers.delete", serverID, "failure", err.Error(), nil)
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_connection_delete_failed"})
		return
	}
	h.recordAudit(r, "servers.delete", serverID, "success", "", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) recordAudit(r *http.Request, action string, resourceID string, result string, message string, metadata map[string]any) {
	user, _ := auth.UserFromContext(r.Context())
	audit.RecordRequest(h.audit, r, audit.Entry{
		Actor: audit.Actor{
			ID:    user.ID,
			Email: user.Email,
			Role:  user.Role,
		},
		Action:     action,
		Resource:   "servers",
		ResourceID: resourceID,
		Result:     result,
		Message:    message,
		Metadata:   metadata,
	})
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
