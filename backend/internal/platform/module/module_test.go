package module

import (
	"context"
	"testing"
	"time"

	"ov-dash/backend/internal/platform/capability"
	"ov-dash/backend/internal/platform/settings"
	"ov-dash/backend/internal/queue"
)

func TestCatalogDescribesRegisteredContract(t *testing.T) {
	catalog, err := NewCatalog(Context{}, contractModule{})
	if err != nil {
		t.Fatal(err)
	}
	items := catalog.Descriptors()
	if len(items) != 1 {
		t.Fatalf("descriptors = %d, want 1", len(items))
	}
	item := items[0]
	if item.ID != "contract" || item.Title != "Contract" || item.Kind != "platform" {
		t.Fatalf("descriptor = %#v", item)
	}
	if len(item.Capabilities) != 1 || item.Capabilities[0].ID != capability.JobsRead {
		t.Fatalf("capabilities = %#v", item.Capabilities)
	}
	if len(item.Settings) != 1 || item.Settings[0].Key != "contract.enabled" {
		t.Fatalf("settings = %#v", item.Settings)
	}
	if len(item.HealthChecks) != 1 || item.HealthChecks[0].ID != "contract.health" {
		t.Fatalf("health = %#v", item.HealthChecks)
	}
	if len(item.Jobs) != 1 || item.Jobs[0].Type != "contract.job" || item.Jobs[0].ModuleID != "contract" {
		t.Fatalf("jobs = %#v", item.Jobs)
	}
}

func TestCatalogRejectsInvalidTopologyAndConflicts(t *testing.T) {
	tests := []struct {
		name    string
		modules []Module
	}{
		{name: "duplicate module", modules: []Module{emptyModule{id: "same"}, emptyModule{id: "same"}}},
		{name: "missing dependency", modules: []Module{emptyModule{id: "child", dependencies: []string{"missing"}}}},
		{name: "dependency cycle", modules: []Module{emptyModule{id: "a", dependencies: []string{"b"}}, emptyModule{id: "b", dependencies: []string{"a"}}}},
		{name: "job conflict", modules: []Module{jobModule{id: "a", jobType: "shared"}, jobModule{id: "b", jobType: "shared"}}},
		{name: "capability conflict", modules: []Module{capabilityModule{id: "a"}, capabilityModule{id: "b"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewCatalog(Context{}, test.modules...); err == nil {
				t.Fatal("expected catalog construction to fail")
			}
		})
	}
}

type contractModule struct{}

func (contractModule) Manifest() Manifest {
	return Manifest{ID: "contract", Title: "Contract", Kind: "platform", Tags: []string{"platform", "jobs"}}
}

func (contractModule) Register(reg *Registrar) error {
	if err := reg.Capabilities(capability.JobsRead); err != nil {
		return err
	}
	if err := reg.Settings(settings.Schema{Key: "contract.enabled", Type: settings.TypeBoolean}); err != nil {
		return err
	}
	if err := reg.Health(HealthCheck{ID: "contract.health", Name: "Contract", Check: func(context.Context) error { return nil }}); err != nil {
		return err
	}
	return reg.Job(JobDefinition{Type: "contract.job", Timeout: 5 * time.Second, MaxAttempts: 3, Handler: noopHandler{}})
}

type emptyModule struct {
	id           string
	dependencies []string
}

func (m emptyModule) Manifest() Manifest      { return Manifest{ID: m.id, DependsOn: m.dependencies} }
func (emptyModule) Register(*Registrar) error { return nil }

type jobModule struct{ id, jobType string }

func (m jobModule) Manifest() Manifest { return Manifest{ID: m.id} }
func (m jobModule) Register(reg *Registrar) error {
	return reg.Job(JobDefinition{Type: m.jobType, Handler: noopHandler{}})
}

type capabilityModule struct{ id string }

func (m capabilityModule) Manifest() Manifest          { return Manifest{ID: m.id} }
func (capabilityModule) Register(reg *Registrar) error { return reg.Capabilities(capability.JobsRead) }

type noopHandler struct{}

func (noopHandler) Handle(context.Context, queue.Job) error { return nil }
