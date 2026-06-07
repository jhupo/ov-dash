package capability

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type Capability string

const (
	UpdatesManage Capability = "updates:manage"
	JobsCreate    Capability = "jobs:create"
	ServersWrite  Capability = "servers:write"
	ServersDelete Capability = "servers:delete"
	ServersSSH    Capability = "servers:ssh"
)

type User struct {
	Role string
}

type UserResolver func(context.Context) (User, bool)

type RolePolicy struct {
	grants map[string]map[Capability]struct{}
}

func DefaultRolePolicy() *RolePolicy {
	return &RolePolicy{grants: map[string]map[Capability]struct{}{
		"operator": capabilities(
			JobsCreate,
			ServersWrite,
			ServersSSH,
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
