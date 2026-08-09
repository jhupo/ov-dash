package servers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/platform/audit"
	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/httpx"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/platform/redact"

	"github.com/go-chi/chi/v5"
)

type Module struct{}

type Handler struct {
	inventory      *InventoryService
	ssh            *SSHAccessService
	cache          cacheStore
	audit          audit.RequestRecorder
	allowedOrigins []string
}

type cacheStore interface {
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Take(ctx context.Context, key string) (string, error)
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
	ExpiresAt   *string `json:"expires_at"`
	ClearSecret bool    `json:"clear_secret"`
}

type serverCommandRequest struct {
	Command string `json:"command"`
}

func NewModule() Module {
	return Module{}
}

func (Module) Manifest() platformmodule.Manifest {
	return platformmodule.Manifest{
		ID:          "servers",
		Title:       "Servers",
		Description: "Server inventory, credential storage, SSH commands, and WebSSH access.",
		Kind:        "service",
		Tags:        []string{"servers", "inventory", "ssh"},
	}
}

func (Module) Register(reg *platformmodule.Registrar) error {
	if err := reg.HTTP(func(ctx platformmodule.Context) {
		repository := NewRepository(ctx.DB, ctx.Secrets)
		handler := &Handler{
			inventory:      NewInventoryService(repository),
			ssh:            NewSSHAccessService(repository, nil, time.Now),
			cache:          ctx.Cache,
			audit:          ctx.Audit,
			allowedOrigins: append([]string(nil), ctx.Config.HTTP.AllowedOrigins...),
		}
		readServers := ctx.RequireCapability(capability.ServersRead)
		writeServers := ctx.RequireCapability(capability.ServersWrite)
		deleteServers := ctx.RequireCapability(capability.ServersDelete)
		sshServers := ctx.RequireCapability(capability.ServersSSH)

		ctx.ProtectedRouter.With(readServers).Get("/server-connections", handler.List)
		ctx.ProtectedRouter.With(readServers).Get("/server-connections/{id}", handler.Get)
		ctx.ProtectedRouter.With(writeServers).Post("/server-connections", handler.Save)
		ctx.ProtectedRouter.With(writeServers).Put("/server-connections/{id}", handler.Save)
		ctx.ProtectedRouter.With(sshServers).Post("/server-connections/{id}/ssh/command", handler.RunCommand)
		ctx.ProtectedRouter.With(sshServers).Post("/server-connections/{id}/ssh/ticket", handler.IssueShellTicket)
		ctx.ProtectedRouter.With(sshServers).Get("/server-connections/{id}/ssh/ws", handler.Shell)
		ctx.ProtectedRouter.With(deleteServers).Delete("/server-connections/{id}", handler.Delete)
	}); err != nil {
		return err
	}
	return reg.Capabilities(
		capability.ServersRead,
		capability.ServersWrite,
		capability.ServersDelete,
		capability.ServersSSH,
	)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.inventory.List(r.Context())
	if err != nil {
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_connections_list_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	item, err := h.inventory.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if IsNotFound(err) {
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "server_connection_not_found"})
			return
		}
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_connection_get_failed"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, item.Public())
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

	item, err := h.inventory.Save(r.Context(), SaveInput{
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
		ExpiresAt:   expiresAt,
		ClearSecret: payload.ClearSecret,
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
	output, err := h.ssh.RunCommand(ctx, serverID, command)
	if err != nil {
		code := sshFailureCode(err)
		h.recordAudit(r, "servers.ssh.command", serverID, "failure", code, metadata)
		status := http.StatusBadGateway
		if IsNotFound(err) {
			status = http.StatusNotFound
		}
		httpx.WriteJSON(w, status, map[string]string{
			"error":  code,
			"output": output,
		})
		return
	}
	h.recordAudit(r, "servers.ssh.command", serverID, "success", "", metadata)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"output": output})
}

func (h *Handler) IssueShellTicket(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	user, ok := auth.UserFromContext(r.Context())
	if !ok || strings.TrimSpace(user.ID) == "" {
		httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.cache == nil {
		h.recordAudit(r, "servers.ssh.ticket", serverID, "failure", "ssh_ticket_store_unavailable", nil)
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "ssh_ticket_store_unavailable"})
		return
	}
	if serverID == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_server_id"})
		return
	}
	if _, err := h.inventory.Get(r.Context(), serverID); err != nil {
		if IsNotFound(err) {
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "server_connection_not_found"})
			return
		}
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_connection_get_failed"})
		return
	}
	ticket, err := randomTicket()
	if err != nil {
		h.recordAudit(r, "servers.ssh.ticket", serverID, "failure", err.Error(), nil)
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "ssh_ticket_create_failed"})
		return
	}
	expiresIn, expiresAt := shellTicketExpiry(time.Now())
	ticketValue, err := encodeShellTicket(shellTicket{ServerID: serverID, UserID: user.ID})
	if err != nil {
		h.recordAudit(r, "servers.ssh.ticket", serverID, "failure", "ssh_ticket_create_failed", nil)
		httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "ssh_ticket_create_failed"})
		return
	}
	if err := h.cache.Set(r.Context(), shellTicketKey(ticket), ticketValue, expiresIn); err != nil {
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
	user, ok := auth.UserFromContext(r.Context())
	if !ok || strings.TrimSpace(user.ID) == "" {
		httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if !isWebSocketUpgradeRequest(r) {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "websocket_upgrade_required"})
		return
	}
	if !webSocketOriginAllowed(r, h.allowedOrigins) {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "websocket_origin_forbidden"})
		return
	}
	if err := h.consumeShellTicket(r.Context(), serverID, user.ID, r.URL.Query().Get("ticket")); err != nil {
		httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	ws, err := upgradeWebSocket(w, r, h.allowedOrigins)
	if err != nil {
		return
	}
	defer ws.Close()
	h.serveShell(r.Context(), ws, serverID)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if err := h.inventory.Delete(r.Context(), serverID); err != nil {
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
