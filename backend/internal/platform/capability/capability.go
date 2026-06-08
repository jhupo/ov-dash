package capability

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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

	UpdatesManage Capability = UpdatesApply
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

type RolePolicy struct {
	grants map[string]map[Capability]struct{}
}

func DefaultRolePolicy() *RolePolicy {
	return &RolePolicy{grants: map[string]map[Capability]struct{}{
		"viewer": capabilities(
			DashboardRead,
			PlatformRead,
			TasksRead,
			AppsRead,
			ChatsRead,
			UsersRead,
			WikiRead,
			SettingsRead,
			ProxyRead,
			NotificationsRead,
			UpdatesRead,
			JobsRead,
			ServersRead,
		),
		"operator": capabilities(
			DashboardRead,
			PlatformRead,
			TasksRead,
			TasksWrite,
			AppsRead,
			ChatsRead,
			UsersRead,
			WikiRead,
			WikiWrite,
			SettingsRead,
			ProxyRead,
			NotificationsRead,
			UpdatesRead,
			JobsRead,
			JobsCreate,
			JobsManage,
			ServersRead,
			ServersWrite,
			ServersSSH,
			ProxyWrite,
			NotificationsWrite,
		),
	}}
}

func (p *RolePolicy) Allows(role string, capability Capability) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "admin" {
		return true
	}
	if p == nil {
		return false
	}
	grants, ok := p.grants[role]
	if !ok {
		return false
	}
	_, ok = grants[capability]
	return ok
}

func RequireCapability(policy *RolePolicy, resolve UserResolver, capability Capability) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := resolve(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			if !policy.Allows(user.Role, capability) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func capabilities(values ...Capability) map[Capability]struct{} {
	result := make(map[Capability]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
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
