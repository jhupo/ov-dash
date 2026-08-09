package secret

import (
	"context"
	"errors"
	"testing"

	"ov-dash/backend/internal/db"
)

func TestNilStoreMethodsDoNotPanic(t *testing.T) {
	var store *Store
	if _, err := store.Put(context.Background(), "scope", "name", "value"); err == nil {
		t.Fatal("nil Store.Put returned no error")
	}
	if _, err := store.Get(context.Background(), "sec_id"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nil Store.Get error = %v, want ErrNotFound", err)
	}
	if err := store.Delete(context.Background(), "sec_id"); err != nil {
		t.Fatalf("nil Store.Delete error = %v", err)
	}
}

func TestNewStoreRejectsUnknownActiveKey(t *testing.T) {
	_, err := NewStore(&db.Pool{}, "next", map[string]string{"current": "secret"})
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("expected ErrUnknownKey, got %v", err)
	}
}

func TestNewStoreBuildsKeyring(t *testing.T) {
	store, err := NewStore(&db.Pool{}, "next", map[string]string{
		"current": "old-secret",
		"next":    "new-secret",
	})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if store.activeKeyID != "next" || len(store.keys) != 2 {
		t.Fatalf("unexpected keyring: active=%q keys=%d", store.activeKeyID, len(store.keys))
	}
}
