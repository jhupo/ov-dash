package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ov-dash/backend/internal/updater"
)

func TestParseFlagsUsesReadOnlyValidationDefaults(t *testing.T) {
	options, err := parseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	validation, err := parseList(options.validationSvcs)
	if err != nil {
		t.Fatal(err)
	}
	resume, err := parseList(options.resumeSvcs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(validation, []string{"api"}) || !reflect.DeepEqual(resume, []string{"worker", "frontend"}) {
		t.Fatalf("validation = %v resume = %v", validation, resume)
	}
	if options.validationURL != "http://127.0.0.1:8080/readyz?scope=api" {
		t.Fatalf("validation URL = %q", options.validationURL)
	}
	artifacts, err := parseMapping(options.artifacts)
	if err != nil {
		t.Fatal(err)
	}
	services, err := parseMapping(options.services)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(artifacts, map[string]string{"backend": "BACKEND_IMAGE", "frontend": "FRONTEND_IMAGE"}) || services["migrate"] != "backend" {
		t.Fatalf("artifacts = %v services = %v", artifacts, services)
	}
}

func TestAssertStartSafeIsReadOnlyAndRejectsBlockingStates(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := assertStartSafe(missing); err == nil {
		t.Fatal("assertStartSafe accepted missing state")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only inspection created state: %v", err)
	}

	for _, test := range []struct {
		name   string
		states []updater.State
		want   error
	}{
		{name: "active", states: nil, want: updater.ErrOperationActive},
		{name: "rollback failed", states: []updater.State{updater.StateDownloaded, updater.StateVerified, updater.StatePreflight, updater.StateQuiescing, updater.StateRollingBack, updater.StateRollbackFailed}, want: updater.ErrOperatorInterventionRequired},
		{name: "manual intervention", states: []updater.State{updater.StateDownloaded, updater.StateVerified, updater.StatePreflight, updater.StateQuiescing, updater.StateManualIntervention}, want: updater.ErrOperatorInterventionRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := updater.NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			operation, err := store.Create(updater.Operation{ID: "0123456789abcdef0123456789abcdef", ReleaseID: "ov-dash-2.0.0", State: updater.StateRequested})
			if err != nil {
				t.Fatal(err)
			}
			for _, state := range test.states {
				operation, err = store.Update(operation, func(next *updater.Operation) error {
					next.State = state
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := assertStartSafe(store.Root()); !errors.Is(err, test.want) {
				t.Fatalf("assertStartSafe() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestRunAssertStartSafeDoesNotRequireReleaseConfiguration(t *testing.T) {
	store, err := updater.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--assert-start-safe", "--state-dir", store.Root()}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
}
