package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type Capability string

const (
	DashboardRead      Capability = "dashboard:read"
	PlatformRead       Capability = "platform:read"
	TasksRead          Capability = "tasks:read"
	TasksWrite         Capability = "tasks:write"
	AppsRead           Capability = "apps:read"
	ChatsRead          Capability = "chats:read"
	UsersRead          Capability = "users:read"
	WikiRead           Capability = "wiki:read"
	WikiWrite          Capability = "wiki:write"
	SettingsRead       Capability = "settings:read"
	SettingsWrite      Capability = "settings:write"
	ProxyRead          Capability = "proxy:read"
	ProxyWrite         Capability = "proxy:write"
	NotificationsRead  Capability = "notifications:read"
	NotificationsWrite Capability = "notifications:write"
	UpdatesRead        Capability = "updates:read"
	UpdatesApply       Capability = "updates:apply"
	JobsRead           Capability = "jobs:read"
	JobsCreate         Capability = "jobs:create"
	JobsManage         Capability = "jobs:manage"
	ServersRead        Capability = "servers:read"
	ServersWrite       Capability = "servers:write"
	ServersDelete      Capability = "servers:delete"
	ServersSSH         Capability = "servers:ssh"
)

type User struct {
	Role string
}

type Descriptor struct {
	ID       Capability `json:"id"`
	Resource string     `json:"resource"`
	Action   string     `json:"action"`
}

type UserResolver func(context.Context) (User, bool)

type Authorizer interface {
	Authorize(context.Context, User, Capability) (bool, error)
}

type QueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type PostgresAuthorizer struct {
	db QueryRower
}

func NewPostgresAuthorizer(db QueryRower) *PostgresAuthorizer {
	return &PostgresAuthorizer{db: db}
}

func (a *PostgresAuthorizer) Authorize(ctx context.Context, user User, capability Capability) (bool, error) {
	role := normalizeRole(user.Role)
	if role == "admin" {
		return true, nil
	}
	if a == nil || a.db == nil || role == "" || strings.TrimSpace(string(capability)) == "" {
		return false, nil
	}

	var allowed bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM role_capabilities
			WHERE role = $1 AND capability = $2
		)
	`, role, capability).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("authorize capability: %w", err)
	}
	return allowed, nil
}

func RequireCapability(authorizer Authorizer, resolve UserResolver, capability Capability) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := resolve(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			if authorizer == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authorization_failed"})
				return
			}
			allowed, err := authorizer.Authorize(r.Context(), user, capability)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authorization_failed"})
				return
			}
			if !allowed {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func Describe(value Capability) Descriptor {
	parts := strings.SplitN(string(value), ":", 2)
	descriptor := Descriptor{ID: value}
	if len(parts) > 0 {
		descriptor.Resource = parts[0]
	}
	if len(parts) > 1 {
		descriptor.Action = parts[1]
	}
	return descriptor
}

func Descriptors(values []Capability) []Descriptor {
	descriptors := make([]Descriptor, 0, len(values))
	for _, value := range values {
		descriptors = append(descriptors, Describe(value))
	}
	return descriptors
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
