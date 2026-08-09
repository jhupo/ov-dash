package servers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/platform/audit"
	"ov-dash/backend/internal/platform/capability"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/platform/redact"

	"github.com/go-chi/chi/v5"
)

func TestRecordAuditCapturesActorRequestAndMetadata(t *testing.T) {
	recorder := &fakeAuditRecorder{}
	handler := &Handler{audit: recorder}
	request := httptest.NewRequest("POST", "/server-connections/srv-1/ssh/command", nil)
	request = request.WithContext(auth.ContextWithUser(request.Context(), auth.User{
		ID:    "user-1",
		Email: "ops@example.com",
		Role:  "admin",
	}))
	request.RemoteAddr = "198.51.100.7:43210"
	request.Header.Set("User-Agent", "servers-test")
	request.Header.Set("X-Forwarded-For", "203.0.113.20, 192.0.2.10")

	handler.recordAudit(request, "servers.ssh.command", "srv-1", "success", "", map[string]any{
		"command":        redact.Text("deploy password=super-secret"),
		"command_length": len("deploy password=super-secret"),
	})

	entry := recorder.single(t)
	if entry.Action != "servers.ssh.command" || entry.Resource != "servers" || entry.ResourceID != "srv-1" || entry.Result != "success" {
		t.Fatalf("audit entry did not describe server command: %#v", entry)
	}
	if entry.Actor.ID != "user-1" || entry.Actor.Email != "ops@example.com" || entry.Actor.Role != "admin" {
		t.Fatalf("audit actor = %#v", entry.Actor)
	}
	if entry.IP != "198.51.100.7" || entry.UserAgent != "servers-test" {
		t.Fatalf("request audit fields = ip %q ua %q", entry.IP, entry.UserAgent)
	}
	if entry.Metadata["command"] == "deploy password=super-secret" {
		t.Fatalf("command metadata was not redacted: %#v", entry.Metadata)
	}
	if entry.Metadata["command_length"] != len("deploy password=super-secret") {
		t.Fatalf("command length metadata = %#v", entry.Metadata["command_length"])
	}
}

func TestManifestOnlyDescribesInventoryAndSSH(t *testing.T) {
	var module platformmodule.Module = Module{}
	manifest := module.Manifest()
	if manifest.ID != "servers" {
		t.Fatalf("module id = %q, want servers", manifest.ID)
	}
	text := strings.ToLower(manifest.Description + " " + strings.Join(manifest.Tags, " "))
	for _, removed := range []string{"agent", "probe", "monitor", "metric", "collector"} {
		if strings.Contains(text, removed) {
			t.Fatalf("manifest still contains removed subsystem %q: %#v", removed, manifest)
		}
	}
}

func TestModuleRegistersServerCapabilities(t *testing.T) {
	catalog, err := platformmodule.NewCatalog(platformmodule.Context{}, Module{})
	if err != nil {
		t.Fatalf("register servers module: %v", err)
	}
	descriptors := catalog.Descriptors()
	if len(descriptors) != 1 || len(descriptors[0].Capabilities) != 4 {
		t.Fatalf("server registration = %#v", descriptors)
	}
}

func TestWebSSHRouteUsesProtectedSSHCapability(t *testing.T) {
	publicRouter := chi.NewRouter()
	protectedRouter := chi.NewRouter()
	moduleContext := platformmodule.Context{
		PublicRouter:    publicRouter,
		ProtectedRouter: protectedRouter,
		RequireCapability: func(required capability.Capability) func(http.Handler) http.Handler {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if required != capability.ServersSSH {
						next.ServeHTTP(w, r)
						return
					}
					w.WriteHeader(http.StatusTeapot)
				})
			}
		},
	}
	catalog, err := platformmodule.NewCatalog(moduleContext, Module{})
	if err != nil {
		t.Fatalf("create module catalog: %v", err)
	}
	catalog.RegisterHTTP(moduleContext)

	path := "/server-connections/srv_1/ssh/ws?ticket=one-time"
	publicResponse := httptest.NewRecorder()
	publicRouter.ServeHTTP(publicResponse, httptest.NewRequest(http.MethodGet, path, nil))
	if publicResponse.Code != http.StatusNotFound {
		t.Fatalf("public route status = %d, want 404", publicResponse.Code)
	}

	protectedResponse := httptest.NewRecorder()
	protectedRouter.ServeHTTP(protectedResponse, httptest.NewRequest(http.MethodGet, path, nil))
	if protectedResponse.Code != http.StatusTeapot {
		t.Fatalf("protected route status = %d, want capability middleware response", protectedResponse.Code)
	}
}

func TestWebSocketOriginPolicy(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		allowedOrigins []string
		want           bool
	}{
		{name: "same origin", origin: "https://dash.example.com", want: true},
		{name: "configured origin", origin: "https://admin.example.com", allowedOrigins: []string{"https://admin.example.com/"}, want: true},
		{name: "cross origin", origin: "https://attacker.example.com", want: false},
		{name: "wildcard is not trusted", origin: "https://attacker.example.com", allowedOrigins: []string{"*"}, want: false},
		{name: "missing origin", want: false},
		{name: "origin with path", origin: "https://dash.example.com/path", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://dash.example.com/api/v1/server-connections/srv/ssh/ws", nil)
			request.Host = "dash.example.com"
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if got := webSocketOriginAllowed(request, test.allowedOrigins); got != test.want {
				t.Fatalf("webSocketOriginAllowed() = %t, want %t", got, test.want)
			}
		})
	}
}

type fakeAuditRecorder struct {
	entries []audit.Entry
}

func (r *fakeAuditRecorder) Record(_ context.Context, entry audit.Entry) error {
	r.entries = append(r.entries, entry)
	return nil
}

func (r *fakeAuditRecorder) single(t *testing.T) audit.Entry {
	t.Helper()
	if len(r.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1: %#v", len(r.entries), r.entries)
	}
	return r.entries[0]
}
