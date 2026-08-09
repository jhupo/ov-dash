package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	platformmodule "ov-dash/backend/internal/platform/module"
)

func TestSessionCookieSecureIsExplicit(t *testing.T) {
	response := httptest.NewRecorder()
	setSessionCookie(response, "token", time.Now().Add(time.Hour), true)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie = %#v", cookies)
	}
}

func TestModuleUsesUnifiedContract(t *testing.T) {
	var module platformmodule.Module = Module{}
	if module.Manifest().ID != "auth" {
		t.Fatalf("module id = %q, want auth", module.Manifest().ID)
	}
	if _, err := platformmodule.NewCatalog(platformmodule.Context{}, module); err != nil {
		t.Fatalf("register auth module: %v", err)
	}
}

func TestMeIncludesCapabilities(t *testing.T) {
	repository := &fakeAuthRepository{
		capabilities: []string{"jobs:manage", "servers:ssh"},
	}
	request := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	request = request.WithContext(ContextWithUser(request.Context(), User{
		ID:   "user-1",
		Role: "operator",
	}))
	response := httptest.NewRecorder()

	(&Handler{service: NewService(repository)}).Me(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload struct {
		User User `json:"user"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.User.Capabilities) != 2 || payload.User.Capabilities[0] != "jobs:manage" {
		t.Fatalf("capabilities = %#v", payload.User.Capabilities)
	}
	if len(repository.capabilityRoles) != 1 || repository.capabilityRoles[0] != "operator" {
		t.Fatalf("capability roles = %#v", repository.capabilityRoles)
	}
}
