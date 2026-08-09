package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ov-dash/backend/internal/platform/capability"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/platform/settings"
	"ov-dash/backend/internal/queue"
)

func TestPlatformModulesHandlerReturnsManifestContracts(t *testing.T) {
	catalog, err := platformmodule.NewCatalog(platformmodule.Context{}, testPlatformModule{})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	handler := platformModulesHandler(catalog)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/platform/modules", nil)
	handler(recorder, request)

	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, stdhttp.StatusOK, recorder.Body.String())
	}
	var body struct {
		Items        []moduleManifestResponse `json:"items"`
		Capabilities []capability.Descriptor  `json:"capabilities"`
		Jobs         []jobDefinitionResponse  `json:"jobs"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items = %#v, want one module", body.Items)
	}
	item := body.Items[0]
	if item.ID != "test" || item.Title != "Test Module" || item.Kind != "platform" {
		t.Fatalf("module identity = %#v", item)
	}
	if len(item.Capabilities) != 1 || item.Capabilities[0].ID != capability.PlatformRead {
		t.Fatalf("module capabilities = %#v", item.Capabilities)
	}
	if len(item.Settings) != 1 || item.Settings[0].Key != "test.enabled" {
		t.Fatalf("module settings = %#v", item.Settings)
	}
	if len(item.HealthChecks) != 1 || item.HealthChecks[0].ID != "test.health" {
		t.Fatalf("module health checks = %#v", item.HealthChecks)
	}
	if len(item.Jobs) != 1 || item.Jobs[0].Type != "test.job" || item.Jobs[0].ModuleID != "test" {
		t.Fatalf("module jobs = %#v", item.Jobs)
	}
	if len(body.Jobs) != 1 || body.Jobs[0].Type != "test.job" || body.Jobs[0].ModuleID != "test" {
		t.Fatalf("top-level jobs = %#v", body.Jobs)
	}
}

type testPlatformModule struct{}

func (testPlatformModule) Manifest() platformmodule.Manifest {
	return platformmodule.Manifest{
		ID:          "test",
		Title:       "Test Module",
		Description: "Test module contract.",
		Kind:        "platform",
		Tags:        []string{"test"},
	}
}

func (testPlatformModule) Register(registrar *platformmodule.Registrar) error {
	if err := registrar.Capabilities(capability.PlatformRead); err != nil {
		return err
	}
	if err := registrar.Settings(settings.Schema{Key: "test.enabled", Type: settings.TypeBoolean}); err != nil {
		return err
	}
	if err := registrar.Health(platformmodule.HealthCheck{
		ID:   "test.health",
		Name: "Test health",
		Check: func(context.Context) error {
			return nil
		},
	}); err != nil {
		return err
	}
	return registrar.Job(platformmodule.JobDefinition{
		Type:        "test.job",
		Description: "Test job.",
		Timeout:     2 * time.Second,
		MaxAttempts: 2,
		Handler:     testJobHandler{},
	})
}

type testJobHandler struct{}

func (testJobHandler) Handle(context.Context, queue.Job) error {
	return nil
}
