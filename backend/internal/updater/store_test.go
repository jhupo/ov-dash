package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStorePersistsContiguousImmutableEvents(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	operation, err := store.Create(Operation{ID: "0123456789abcdef0123456789abcdef", ReleaseID: "ov-dash-2.0.0", State: StateRequested})
	if err != nil {
		t.Fatal(err)
	}
	operation, err = store.Update(operation, func(next *Operation) error {
		next.State = StateDownloaded
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.operationDir(operation.ID), ".tmp-interrupted"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(store.Root())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Get(operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StateDownloaded || loaded.Revision != 2 {
		t.Fatalf("loaded = state %s revision %d", loaded.State, loaded.Revision)
	}
	events, err := reopened.Events(operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Previous != StateRequested {
		t.Fatalf("events = %#v", events)
	}

	stale := operation
	stale.Revision = 1
	if _, err := reopened.Update(stale, func(*Operation) error { return nil }); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale Update() error = %v, want revision conflict", err)
	}
}

func TestStoreRejectsInvalidTransitionAndJournalGap(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	operation, err := store.Create(Operation{ID: "1123456789abcdef0123456789abcdef", ReleaseID: "ov-dash-2.0.0", State: StateRequested})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(operation, func(next *Operation) error {
		next.State = StateCommitted
		return nil
	}); err == nil {
		t.Fatal("invalid transition succeeded")
	}
	if err := os.WriteFile(filepath.Join(store.operationDir(operation.ID), "00000000000000000003.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(operation.ID); err == nil {
		t.Fatal("journal gap was accepted")
	}
}
