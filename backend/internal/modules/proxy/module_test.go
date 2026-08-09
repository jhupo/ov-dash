package proxy

import (
	"testing"

	"ov-dash/backend/internal/platform/capability"
	platformmodule "ov-dash/backend/internal/platform/module"
)

func TestModuleUsesUnifiedContract(t *testing.T) {
	var module platformmodule.Module = Module{}
	catalog, err := platformmodule.NewCatalog(platformmodule.Context{}, module)
	if err != nil {
		t.Fatalf("register proxy module: %v", err)
	}
	descriptors := catalog.Descriptors()
	if len(descriptors) != 1 || descriptors[0].ID != "proxy" {
		t.Fatalf("module descriptors = %#v", descriptors)
	}
	if len(descriptors[0].Capabilities) != 2 || len(descriptors[0].Settings) != 5 {
		t.Fatalf("proxy registration = %#v", descriptors[0])
	}
	if descriptors[0].Capabilities[0].ID != capability.ProxyRead {
		t.Fatalf("first capability = %q, want %q", descriptors[0].Capabilities[0].ID, capability.ProxyRead)
	}
}
