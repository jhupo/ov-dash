package app

import (
	"testing"

	"ov-dash/backend/internal/platform"
	platformmodule "ov-dash/backend/internal/platform/module"
)

func TestBuildRejectsMissingDependencies(t *testing.T) {
	if _, err := build(nil); err == nil {
		t.Fatal("expected missing runtime error")
	}
	if _, err := build(&platform.Runtime{}, invalidModule{}); err == nil {
		t.Fatal("expected invalid module error")
	}
}

type invalidModule struct{}

func (invalidModule) Manifest() platformmodule.Manifest        { return platformmodule.Manifest{} }
func (invalidModule) Register(*platformmodule.Registrar) error { return nil }
